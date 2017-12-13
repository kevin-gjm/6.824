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
	"bytes"
	"encoding/gob"
	"labrpc"
	"math/rand"
	"sync"
	"time"
	//"fmt"
)

// import "bytes"
// import "encoding/gob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
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
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm int
	votedFor    int
	log         []LogEntry

	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int

	grantedVotesCount int
	state             string
	applyCh           chan ApplyMsg

	timer *time.Timer
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	term = rf.currentTerm
	isleader = (rf.state == LEADER)
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(rf.currentTerm)
	enc.Encode(rf.votedFor)
	enc.Encode(rf.log)
	rf.persister.SaveRaftState(buf.Bytes())
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	if data == nil || len(data) < 1 { // bootstrap without any state?

	}
	if data != nil {
		buf := bytes.NewBuffer(data)
		dec := gob.NewDecoder(buf)
		dec.Decode(&rf.currentTerm)
		dec.Decode(&rf.votedFor)
		dec.Decode(&rf.log)
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	//fmt.Printf("%v receive request vote for %v\n", rf.me, args.CandidateID)
	rf.mu.Lock()
	defer rf.mu.Unlock()

	mayGantVote := true
	if len(rf.log) > 0 {
		if rf.log[len(rf.log)-1].Term > args.LastLogTerm ||
			(rf.log[len(rf.log)-1].Term == args.LastLogTerm && len(rf.log)-1 > args.LastLogIndex) {
			mayGantVote = false
		}
	}

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.currentTerm {
		rf.state = FOLLOWER
		rf.currentTerm = args.Term

		reply.Term = args.Term

		if mayGantVote {
			rf.votedFor = args.CandidateID
			reply.VoteGranted = true
		} else {
			rf.votedFor = -1
			reply.VoteGranted = false
		}
		rf.resetTimer()
		rf.persist()
		return
	}

	if args.Term == rf.currentTerm {
		if rf.votedFor == -1 && mayGantVote {
			rf.votedFor = args.CandidateID
			reply.VoteGranted = true
			rf.resetTimer()
		} else {
			reply.VoteGranted = false
		}
		reply.Term = args.Term
		return
	}
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
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
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
	rf.mu.Lock()
	defer rf.mu.Unlock()

	index := -1
	term := -1
	isLeader := false

	// Your code here (2B).
	if rf.state != LEADER {
		return index, term, isLeader
	}
	nlog := LogEntry{command, rf.currentTerm}

	isLeader = true
	rf.log = append(rf.log, nlog)
	index = len(rf.log)
	term = rf.currentTerm

	rf.persist()
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
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log = make([]LogEntry, 0)

	//这里用的是0到时候要改变整个logcommit部分
	rf.commitIndex = -1
	rf.lastApplied = -1
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

const (
	FOLLOWER  = "FOLLOWER"
	LEADER    = "LEADER"
	CANDIDATE = "CANDIDATE"
)
const (
	HEARTBEAT       = 50
	ELECTIONTIMEOUT = 100
)

func (rf *Raft) resetTimer() {
	var timeOut int
	rf.timer.Stop()
	if rf.state == LEADER {
		timeOut = HEARTBEAT
	} else {
		timeOut = ELECTIONTIMEOUT + rand.Intn(2*ELECTIONTIMEOUT)
	}
	rf.timer.Reset(time.Duration(timeOut) * time.Millisecond)
}
func (rf *Raft) handleTimer() {
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
			LastLogIndex: len(rf.log) - 1, //方便数组取值(下标从0开始)
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
				//fmt.Printf("%v sendRequestVote to %v\n", rf.me, server)
				ok := rf.sendRequestVote(server, &args, &replay)
				if ok {
					rf.handleVoteResult(replay)
				}
			}(server, args)
		}
	} else {
		//fmt.Printf("%v send heartbeat\n", rf.me)
		rf.sendAppendEntriesToAllFollwer()
	}
}

func (rf *Raft) handleVoteResult(replay RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if replay.Term < rf.currentTerm {
		return
	}
	if replay.Term > rf.currentTerm {
		rf.state = FOLLOWER
		rf.votedFor = -1
		rf.grantedVotesCount = 0
		rf.currentTerm = replay.Term
		rf.resetTimer()
		rf.persist()
		return
	}
	if replay.VoteGranted && rf.state == CANDIDATE {
		rf.grantedVotesCount++
		//fmt.Printf("%v get vote for %v\n", rf.me, rf.grantedVotesCount)
		if rf.grantedVotesCount >= majority(len(rf.peers)) {
			//fmt.Printf("%v become leader------------------->\n", rf.me)
			rf.state = LEADER
			for i := 0; i < len(rf.peers); i++ {
				if i == rf.me {
					continue
				}
				rf.nextIndex[i] = len(rf.log)
				rf.matchIndex[i] = -1
			}
			rf.resetTimer()
			//rf.sendAppendEntriesToAllFollwer()
		}
	}
}

type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PreLogIndex  int
	PreLogTerm   int
	Entries      []LogEntry
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term        int
	Success     bool
	CommitIndex int //日志同步使用
}

func (rf *Raft) sendAppendEntriesToAllFollwer() {
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		args := AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderID:     rf.me,
			PreLogIndex:  rf.nextIndex[i] - 1, //rf成为领导人时，将nextIndex初始化为len(rf.log)
			LeaderCommit: rf.commitIndex,
		}

		if args.PreLogIndex >= 0 {
			args.PreLogTerm = rf.log[args.PreLogIndex].Term
		}
		if rf.nextIndex[i] < len(rf.log) {
			//第一次发送时为nil
			args.Entries = rf.log[rf.nextIndex[i]:]
		}
		go func(server int, args AppendEntriesArgs) {
			var replay AppendEntriesReply
			ok := rf.sendAppendEntries(server, &args, &replay)
			if ok {
				rf.handleAppendEntries(server, replay) //回复中需要对rf中存储server相关状态的字段进行更改，所以传送server的编号
			}
		}(i, args)

	}
}
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//fmt.Printf("%v receive %v 's heartbeat\n", rf.me, args.LeaderID)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.resetTimer()

	if rf.currentTerm > args.Term {
		reply.Term = rf.currentTerm
		reply.Success = false
	} else {
		rf.state = FOLLOWER
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.grantedVotesCount = 0
		reply.Term = rf.currentTerm
		//fmt.Printf("AppendEntries  prelogindex %d,prelogterm:%d,leadercommit:%d\n",args.PreLogIndex,args.PreLogTerm,args.LeaderCommit)
		// 获得正确的term和index返回给leader。与leader的日志长度不一样情况下
		if args.PreLogIndex >= 0 && (len(rf.log)-1 < args.PreLogIndex ||rf.log[args.PreLogIndex].Term != args.PreLogTerm) {
			reply.CommitIndex = len(rf.log) - 1 //本地提交的日志。
			if reply.CommitIndex > args.PreLogIndex {
				reply.CommitIndex = args.PreLogIndex //比leader超前的部分需要丢弃，与leader保持一致
			}
			for reply.CommitIndex >= 0 {
				if rf.log[reply.CommitIndex].Term == args.PreLogTerm {
					break //找到没有冲突的地方
				}
				reply.CommitIndex-- //若不存在与args.PreLogTerm相同的index则从0开始重新开始搞
			}
			reply.Success = false
		} else 
			// preLogIndex/term都是正常的，此时进行log操作
			if args.Entries != nil {
				//进行日志操作
				rf.log = rf.log[:args.PreLogIndex+1]     //前闭后开区间
				rf.log = append(rf.log, args.Entries...) //将后一个slice添加到前一个slice中，只接受两个参数，且需要在最后面加上三个点
				if len(rf.log)-1 >= args.LeaderCommit {
					rf.commitIndex = args.LeaderCommit
					go rf.commitLogs()
				}
				reply.Success = true
				reply.CommitIndex = len(rf.log) - 1 //????   reply.CommitIndex = rf.commitIndex
			} else {
				//没有日志操作，心跳
				if len(rf.log)-1 >= args.LeaderCommit {
					rf.commitIndex = args.LeaderCommit
					go rf.commitLogs()
				}
				reply.Success = true
				reply.CommitIndex = args.PreLogIndex
			}

		
		rf.persist()
	}

}
func (rf *Raft) handleAppendEntries(server int, replay AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//fmt.Printf("%v receive %v 's heartbeat reply\n", rf.me, server)
	if rf.state != LEADER {
		return
	}
	if replay.Term > rf.currentTerm {
		rf.state = FOLLOWER
		rf.currentTerm = replay.Term
		rf.votedFor = -1
		rf.grantedVotesCount = 0
		rf.resetTimer()
		return
	}
	if replay.Success {
		rf.nextIndex[server] = replay.CommitIndex + 1
		rf.matchIndex[server] = replay.CommitIndex
		replay_count := 1
		for i := 0; i < len(rf.peers); i++ {
			if i == rf.me {
				continue
			}
			if rf.matchIndex[i] >= rf.matchIndex[server] {
				replay_count++
			}
		}
		if replay_count >= majority(len(rf.peers)) &&
			rf.commitIndex < rf.matchIndex[server] && //若是已经提交了的就不用管了
			rf.log[rf.matchIndex[server]].Term == rf.currentTerm {
			rf.commitIndex = rf.matchIndex[server]
			go rf.commitLogs()
		}
	} else {
		rf.nextIndex[server] = replay.CommitIndex + 1
		rf.sendAppendEntriesToAllFollwer()
	}
}
func majority(len int) int {
	return len/2 + 1
}
func (rf *Raft) commitLogs() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.commitIndex > len(rf.log)-1 {
		rf.commitIndex = len(rf.log) - 1
	}

	for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
		rf.applyCh <- ApplyMsg{Index: i + 1, Command: rf.log[i].Command}
	}
	rf.lastApplied = rf.commitIndex
}
