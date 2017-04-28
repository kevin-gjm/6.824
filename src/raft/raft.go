package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"labrpc"
	"math/rand"
	"sync"
	"time"
)

// import "bytes"
// import "encoding/gob"

var FOLLOWER = "FOLLOWER"
var LEADER = "LEADER"
var CANDIDATE = "CANDIDATE"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
//对外发送的数据要大写
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}
type LogEntry struct {
	Command interface{}
	Term    int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm int
	votedFor    int // 用下表索引来标记每一个server
	log         []LogEntry

	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

	// granted vote number
	grantedVotesCount int //统计赞成的票数

	// State and applyMsg chan
	state   string
	applyCh chan ApplyMsg

	// Log and Timer
	//logger *log.Logger
	timer *time.Timer
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	term = rf.currentTerm
	isleader = (rf.state == LEADER)
	// Your code here.
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
}

//
//AppendEntry RPC arguments structure.
//
type AppendEntryArgs struct {
	Term         int
	LeaderID     int
	PreLogIndex  int
	PreLogTerm   int
	Entries      []LogEntry
	LeaderCommit int
}

//
//AppendEntry RPC  reply structure.
//
type AppendEntryReply struct {
	Term    int
	Success bool
	//LeaderCommit int
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
	// Your data here.
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
	// Your data here.
	Term        int
	VoteGranted bool
	//LeaderCommit int
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	//收到投票请求后的处理函数
	//fmt.Printf("RequestVote\n")
	rf.mu.Lock()
	defer rf.mu.Unlock()

	mayGantVote := true

	// term and index <= votefor
	if len(rf.log) > 0 {
		if rf.log[len(rf.log)-1].Term > args.Term ||
			(rf.log[len(rf.log)-1].Term == args.Term && len(rf.log)-1 > args.LastLogIndex) {
			mayGantVote = false
		}
	}

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
	}

	if args.Term == rf.currentTerm {
		if rf.votedFor == -1 && mayGantVote {
			rf.votedFor = args.CandidateID
			rf.persist()
			rf.resetTimer()
		}
		reply.Term = rf.currentTerm
		reply.VoteGranted = (rf.votedFor == args.CandidateID)
		return

	}

	if args.Term > rf.currentTerm {
		rf.state = FOLLOWER
		rf.votedFor = -1
		rf.currentTerm = args.Term

		if mayGantVote {
			rf.votedFor = args.CandidateID
			rf.persist()
		}
		rf.resetTimer()
		reply.Term = args.Term
		reply.VoteGranted = (rf.votedFor == args.CandidateID)
		return
	}

}

func (rf *Raft) RequestAppendEntries(args AppendEntryArgs, reply *AppendEntryReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
	} else {
		rf.state = FOLLOWER
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.grantedVotesCount = 0
		reply.Term = rf.currentTerm
		reply.Success = true
		rf.resetTimer()
	}
	rf.persist()

}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}
func (rf *Raft) sendRequestAppendEntries(server int, args AppendEntryArgs, reply *AppendEntryReply) bool {
	ok := rf.peers[server].Call("Raft.RequestAppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}
func (rf *Raft) resetTimer() {
	rf.timer.Stop()
	var timeOut int
	if rf.state == LEADER {
		timeOut = 20
	} else {
		electiontimeout := 100
		timeOut = electiontimeout + rand.Intn(2*electiontimeout)
	}
	rf.timer.Reset(time.Duration(timeOut) * time.Millisecond)
}

// 处理超时
func (rf *Raft) handleTimer() {
	// Your code here, if desired.
	//根据之前不同的状态决定之后的状态
	//非领导人则需要装换状态为候选者，term+1,为自己投票，然后发送请求，要求他人为自己投票
	//领导人发送心跳包维持权威
	rf.mu.Lock()
	defer rf.mu.Unlock()

	rf.resetTimer()

	if rf.state != LEADER {
		rf.state = CANDIDATE
		rf.currentTerm++
		rf.votedFor = rf.me
		rf.grantedVotesCount = 1

		rf.persist()

		args := RequestVoteArgs{
			Term:         rf.currentTerm,
			CandidateID:  rf.me,
			LastLogIndex: len(rf.log) - 1,
		}

		if len(rf.log) > 0 {
			args.LastLogTerm = rf.log[args.LastLogIndex].Term
		}

		for server := 0; server < len(rf.peers); server++ {
			if server == rf.me {
				continue
			}

			go func(server int, args RequestVoteArgs) {
				var replay RequestVoteReply
				ok := rf.sendRequestVote(server, args, &replay)
				if ok {
					rf.handleVoteResult(replay)
				}
			}(server, args)

		}
	} else {
		// leader
		rf.sendAppendEntriesToAllFollwer()
	}

}

func (rf *Raft) sendAppendEntriesToAllFollwer() {
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		args := AppendEntryArgs{
			Term:         rf.currentTerm,
			LeaderID:     rf.me,
			PreLogIndex:  rf.nextIndex[i] - 1,
			LeaderCommit: rf.commitIndex,
		}

		if args.PreLogIndex >= 0 {
			args.PreLogTerm = rf.log[args.PreLogIndex].Term
		}

		go func(server int, args AppendEntryArgs) {
			var replay AppendEntryReply
			ok := rf.sendRequestAppendEntries(server, args, &replay)
			if ok {
				rf.handleAppendEntries(replay)
			}
		}(i, args)

	}
}

func (rf *Raft) handleAppendEntries(replay AppendEntryReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != LEADER {
		return
	}
	if replay.Term > rf.currentTerm {
		rf.state = FOLLOWER
		rf.currentTerm = replay.Term
		rf.votedFor = -1
		rf.grantedVotesCount = 0
		rf.resetTimer()
	}
}
func (rf *Raft) handleVoteResult(replay RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if replay.Term < rf.currentTerm {
		return
	}

	if replay.Term > rf.currentTerm {
		rf.currentTerm = replay.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
		rf.resetTimer()
		return
	}
	if replay.VoteGranted && rf.state == CANDIDATE {
		rf.grantedVotesCount++
		if rf.grantedVotesCount >= majority(len(rf.peers)) {
			rf.state = LEADER
			//在超时之前一直等待统计收到的选票数，不能写到外面
			rf.resetTimer()
			for i := 0; i < len(rf.peers); i++ {
				if i == rf.me {
					continue
				}
				rf.nextIndex[i] = len(rf.log)
				rf.matchIndex[i] = -1
				//发送心跳
			}

		}
		return
	}

}
func majority(length int) int {
	return length/2 + 1
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
//创建一个后台线程周期性的进行选举操作事宜
//10次收不到心跳认为断开
//老leader挂掉后，在5s内需要选出领导来(可能需要多次选举才能出现结果)

//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.

	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log = make([]LogEntry, 0)

	rf.commitIndex = 0
	rf.lastApplied = 0

	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))

	rf.grantedVotesCount = 0
	rf.state = FOLLOWER
	rf.applyCh = applyCh

	rf.timer = time.NewTimer(1 * time.Millisecond)
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	rf.persist()
	rf.resetTimer()
	go func() {
		for {
			select {
			case <-rf.timer.C:
				rf.handleTimer()
			}
		}
	}()
	return rf
}

////TODO
//重新封装，将中间的代码封装成函数。如发送投票请求
