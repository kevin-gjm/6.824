package raftkv

import (
	"encoding/gob"
	"labrpc"
	"log"
	"raft"
	"sync"
	"time"
)

const Debug = 1

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

//存储到log中的内容
//client+id标识了唯一一个ID
type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Type	int
	Key 	string
	Value	string
	Client	int64
	Id		int64
}
type P_Op struct{
	flag	chan bool
	op 		*Op
}
type RaftKV struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	persister *raft.Persister
	data	map[string]string		//存储的K/V
	pendingOps map[int][] *P_Op		//正在执行的操作
	op_count   map[int64]int64		//每一个client最后一个已经执行的操作。操作是递增的直接比较大小即可
}

const (
	OpPut		=	0
	OpAppend	=	1
	OpGet		=	2
)
const (
	STATUS_LEADER	=	false
	STATUS_FOLLOWER	=	true
)
const TIMEOUT = time.Second * 3

func (kv *RaftKV) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	var op Op
	op.Type=OpGet
	op.Key=args.Key
	op.Client=args.Client
	
	reply.WrongLeader =kv.execOp(op)
	if reply.WrongLeader {
		reply.Err=ErrNoLeader
	}else{
		kv.mu.Lock()
		defer kv.mu.Unlock()
		// value, ok := kv.data[args.Key]
		// if ok {
		// 	reply.Err=OK
		// 	reply.WrongLeader=false
		// 	reply.Value=value
		// }else
		// {
		// 	reply.Err=ErrNoKey
		// 	reply.WrongLeader=false
		// }

		if value, ok := kv.data[args.Key]; ok {
			reply.Value = value
			reply.Err = OK
		} else {
			reply.Err = ErrNoKey
		}
	}
	
}

func (kv *RaftKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	var op Op
	if args.Op == "Put"{
		op.Type=OpPut
	}else{
		op.Type=OpAppend
	}
	op.Key=args.Key
	op.Value=args.Value
	op.Client=args.Client
	op.Id=args.Id

	reply.WrongLeader = kv.execOp(op)
	if reply.WrongLeader {
		reply.Err = ErrNoLeader
	}else{
		reply.Err=OK
	}
}
func (kv *RaftKV) execOp(op Op) bool{
	op_idx,_,is_leader:= kv.rf.Start(op)
	if !is_leader{
		DPrintf("server %v is not the leader",kv.me)
		return STATUS_FOLLOWER
	}

	waiter:= make(chan bool,1)
	DPrintf("Append to pendingOps op_idx:%v op:%+v",op_idx,op)
	kv.mu.Lock()
	kv.pendingOps[op_idx] = append(kv.pendingOps[op_idx],&P_Op{flag:waiter,op:&op})
	kv.mu.Unlock()

	var ok bool
	timer := time.NewTimer(TIMEOUT)
	select{
	case ok=<- waiter:
	case <-timer.C:
		DPrintf("Wait operation apply to state machine exceeds timeout....\n")
		ok=false
	}
	delete(kv.pendingOps,op_idx)

	if !ok{
		DPrintf("Wrong leader\n")
        return STATUS_FOLLOWER
	}
	return STATUS_LEADER
}

//
// the tester calls Kill() when a RaftKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *RaftKV) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots with persister.SaveSnapshot(),
// and Raft should save its state (including log) with persister.SaveRaftState().
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *RaftKV {
	// call gob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	gob.Register(Op{})

	kv := new(RaftKV)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// Your initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	kv.persister=persister
	kv.data=make(map[string]string)
	kv.pendingOps=make(map[int][]*P_Op)
	kv.op_count=make(map[int64]int64)

	go func(){
		for msg:=range kv.applyCh{
			kv.Apply(&msg)
		}
	}()

	return kv
}

func (kv *RaftKV)Apply(msg *raft.ApplyMsg){
	kv.mu.Lock()
	defer kv.mu.Unlock()

	var args Op
	args = msg.Command.(Op)
	
	//op_count记录
	if kv.op_count[args.Client]>= args.Id {
		DPrintf("Duplicate operation\n")
	}else{
		switch args.Type{
		case OpPut:
			DPrintf("Put key/value:%v/%v\n",args.Key,args.Value)
			kv.data[args.Key]=args.Value
		case OpAppend:
			DPrintf("Append key/value:%v/%v\n",args.Key,args.Value)
			kv.data[args.Key]=kv.data[args.Key]+args.Value
		default:
		}
		kv.op_count[args.Client]=args.Id
	}

	for _,i := range kv.pendingOps[msg.Index] {
		if i.op.Client == args.Client && i.op.Id == args.Id {
			DPrintf("true Client:%v %v, Id:%v %v", i.op.Client, args.Client, i.op.Id, args.Id)
			i.flag<- true
		}else{
			DPrintf("false Client:%v %v, Id:%v %v", i.op.Client, args.Client, i.op.Id, args.Id)
			i.flag<-false
		}
	}
}