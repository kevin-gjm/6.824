### mit6.824实验2 raft

整个实验中关于index值的说明：所有的index值都是从0开始的，是根据数组下标进行标识。故一些状态的初始值不能设置为0.应该设置为-1。整个系统中要是用统一的标识进行操作。

#### lab2A

1. 按照实验说明，首先参考raft论文Figure2，对raft.go中的Raft结构图进行填充。

   进行超时选举操作，所以需要一个定时器。

   每一个server都有自己的状态标识。

   每一个server都有可能成leader，所以需要一个计数器统计获得的选票数量

   表中指出log，需要包含状态机执行的命令和Term编号，所以需要一个结构体。命令不知道什么格式，用interface可以代表任意类型。

   ```go
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
   type LogEntry struct {
   	Command interface{}
   	Term    int
   }
   const (
   	FOLLOWER  = "FOLLOWER"
   	LEADER    = "LEADER"
   	CANDIDATE = "CANDIDATE"
   )
   ```

   ​

2. raft选举机制是透过定时器推动的，所以首先实现定时器相关。

   定时器time包主要参考下面这几个关键字：

   > NewTimer Duration Reset Stop Sleep After Tick

   最终选用NewTimer ,Reset ,Stop 来实现自己的定时器。

   论文中选举超时时间100-300ms。心跳时间貌似没有，自己设置为50ms。

   ```go
   //超时处理函数，在Make中开线程死循环处理超时事件
   go func() {
   		for {
   			select {
   			case <-rf.timer.C:
   				rf.handleTimer()
   			}
   		}
   	}()
   ```

   若在未超时时，发生类似收到其他server的投票请求，需要重启启动自己的定时器。

   ```go
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
   ```

3. 超时发生时，根据状态不同进行不同的操作

   若为leader发送心跳或同步包(暂不说明)维持自己的领导人权威或状态(AppendEntries RPC)，既充当心跳也作为服务器间同步信息的通信

   非leader，转换为figure2中Rules for servers指出了followers装换为candidate，然后进行candidate的选举拉票操作。candidate同样再次进行选举拉票。

   选举过程：

   	1. 改变为candidate
   	2.  增加自己的currentTerm
   	3. 给自己投票
   	4. 重置自己的计时器
   	5. 发送拉票请求给其他server(RequestVote RPC)

4. 发送RequestVote  RPC首先完善其发送和接收参数的结构，参考论文figure2

   ```go
   type RequestVoteArgs struct {
   	// Your data here (2A, 2B).
   	Term         int
   	CandidateId  int
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
   ```

5. 超时处理

   ```go
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
   			CandidateId:  rf.me,
   			LastLogIndex: len(rf.log) - 1, //数组下标，从0开始
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
   				ok := rf.sendRequestVote(server, &args, &replay)
   				if ok {
   					rf.handleVoteResult(replay)
   				}
   			}(server, args)
   		}
   	} else {
   		rf.sendAppendEntriesToAllFollwer()
   	}
   }
   ```

6. server需要对RequestVote 信息进行反应。发送方处理其他server的RPC回复信息，接收方处理他人发过来的请求。

   > 接收方处理RequestVote

   首先判断其是否可以成为领导者：

   	1. term,lastlogindex,lastlogterm比较,都成立才能进行下面的逻辑，不成立，忽略请求返回失败。(两种情况，区别是定时器是否重置)
   	2. 成立投票，并重置timer

   ```go
   func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
   	// Your code here (2A, 2B).
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
   			rf.votedFor = args.CandidateId
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
   			rf.votedFor = args.CandidateId
   			reply.VoteGranted = true
   			rf.resetTimer()
   		} else {
   			reply.VoteGranted = false
   		}
   		reply.Term = args.Term
   		return
   	}
   }
   ```

   > 发送方处理相应RPC的返回信息

   1. 成功（term必定相等）统计票数，判断是否获得大多数选票，大多数则成为领导人。重新计时并发送心跳包（有没有必要发送呢？？？），并根据自己日志初始化为其他server服务的字段

   2. 失败，根据term信息确定状态，若term比自己大，变跟随者，重启定时器。比自己小（貌似不存在这种情况）

      ```go

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
      		if rf.grantedVotesCount >= majority(len(rf.peers)) { //此处为大于等于大多数。注意等于号。第一次编写时，没有等于。不能正常通过reelection
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

      func majority(len int) int {
      	return len/2 + 1
      }
      ```

7. 至此选举完成，但是新的领导人并不能持续的维持自己的权威，需要发送心跳包(AppendEntries RPC)来维持权威。

   > 与之前发送投票RPC类似。首先确定其发送和返回参数的结构，然后补全rpc调用相关函数，发送AppendEntries ，最后完成响应和处理

8. AppendEntries 相关结构,参考论文figure2

   ```go
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
   	CommitIndex int //日志同步使用。用于更新nextIndex
   }
   ```

   ​

9. 仿sendRequestVote函数完成AppendEntries  RPC调用

   ```go
   func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
   	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
   	return ok
   }
   ```

10. 发送AppendEntries  RPC

  ```go
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
  				rf.handleAppendEntries(server, replay) //回复中需要对rf中存储server相关状态的字段进行更改，所以传送server的编号。选举用不到server编号
  			}
  		}(i, args)

  	}
  }
  ```

11. 其他server对此RPC的相应函数AppendEntries

    ```go
    func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
    	fmt.Printf("%v receive %v 's heartbeat\n", rf.me, args.LeaderID)
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
    		reply.Success = true
    		rf.persist()
    	}
    }
    ```

12. 发送方对返回结果的处理handleAppendEntries

    ```go
    func (rf *Raft) handleAppendEntries(server int, replay AppendEntriesReply) {
    	rf.mu.Lock()
    	defer rf.mu.Unlock()

    	fmt.Printf("%v receive %v 's heartbeat reply\n", rf.me, server)
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
    }
    ```

13. 至此完成绝大部分的程序编写。然后运行测试.并不能通过测试，不能正常选出一个leader，但是完全正确啊。查看test中checkOneLeader代码发现需要使用GetState函数获取每一个server的状态。完善次函数

    ```go
    go test -run TestInit
    ```

14. GetState

    ```go
    func (rf *Raft) GetState() (int, bool) {
    	var term int
    	var isleader bool
    	// Your code here (2A).
    	term=rf.currentTerm
    	isleader=(rf.state==LEADER)
    	return term, isleader
    }
    ```

15. OK 通过初步测试。然后测试。完成lab2A的编写

    ```go
    go test -run TestReEle
    ```


#### lab2B

1. 根据实验文档的第一步完善Start函数。根据里面说明，

   > adding a new operation to the log;the leader sends the new operation to the other servers in `AppendEntries` RPCs

   Start完成添加针对于leader的操作,添加日志，然后发送给ApendEntries。同步可以通过timer自动的发送RPC实现。start主要完成其添加日志的操作。

   ```go
   func (rf *Raft) Start(command interface{}) (int, int, bool) {
   	rf.mu.Lock()
   	defer rf.mu.Unlock()

   	index := -1
   	term := -1
   	isLeader := true

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
   ```

2. 调整AppendEntries RPC相关部分，加上log。首先需要搞明白里面几个数据结构之间。

   | 参数                     | 含义                                       |
   | ---------------------- | ---------------------------------------- |
   | nextIndex              | 要发给其他服务器的***下一条***日志的index。leader根据自己的log进行初始化，值为len(rf.log)，。log的last log+1 |
   | matchIndex             | 对于每一个服务器，已经复制给他的日志的最高索引值                 |
   | PreLogIndex,PreLogTerm | 客户日志                                     |
   | LeaderCommit           | leader的commitIndex                       |
   | commitIndex            | leader统计大多数日志同步成功后提交到状态机，并增加commitIndex计数 |
   | lastApplied            | 应用到状态机的日志条目索引。与commitIndex有关联            |
   | reply.CommitIndex      | 自定义参数，用于更新leader中的nextindex,matchIndex   |

3. 发送部分

   ```go
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
   ```

   ​

4. 其他server接收处理

   ```go
   func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
   	fmt.Printf("%v receive %v 's heartbeat\n", rf.me, args.LeaderID)
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

   		//获得正确的term和index返回给leader。与leader的日志长度不一样情况下
           //第二个中len(rf.log)-1 < args.PreLogIndex，在接收server log少于leader情况
           //rf.log[args.PreLogIndex].Term != args.PreLogTerm。执行前已经保证PreLogIndex>=len(log)-1.这是||性质。
   		if args.PreLogIndex >= 0 && 
             (len(rf.log)-1 < args.PreLogIndex ||rf.log[args.PreLogIndex].Term != args.PreLogTerm) {
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
               //最终CommitIndex是与args.PreLogTerm相等term的最大index.若不存在相等的Term则commitIndex=0
   			reply.Success = false
   		} else {
   			// preLogIndex/term都是正常的，此时进行log操作
   			if args.Entries != nil {
   				//进行日志操作
   				rf.log = rf.log[:args.PreLogIndex+1]     //前闭后开区间
   				rf.log = append(rf.log, args.Entries...) //将后一个slice添加到前一个slice中，只接受两个参数，且需要在最后面加上三个点
   				if len(rf.log)-1 >= args.LeaderCommit { //args.leaderCommit只有大多数提交后才commit并发送给其他非leader server.所以是>=0
   					rf.commitIndex = args.LeaderCommit 
   					go rf.commitLogs() //应用到状态机
   				}
   				reply.Success = true
   				reply.CommitIndex = len(rf.log) - 1 //用于确定nextIndex使用的
   			} else {
   				//没有日志操作，心跳
   				if len(rf.log)-1 >= args.LeaderCommit {
   					rf.commitIndex = args.LeaderCommit
   					go rf.commitLogs()
   				}
   				reply.Success = true
   				reply.CommitIndex = args.PreLogIndex
   			}

   		}
   		rf.persist()
   	}

   }
   ```

5. 发送方接收反馈并处理

   ```go
   func (rf *Raft) handleAppendEntries(server int, replay AppendEntriesReply) {
   	rf.mu.Lock()
   	defer rf.mu.Unlock()

   	fmt.Printf("%v receive %v 's heartbeat reply\n", rf.me, server)
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
   ```

   ​

6. 将提交日志交付给状态机

   ```go
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
   ```

   ​

7. go test测试: go test -run TestBasicAgree . go test -run TestFailAgree

#### lab2C

1. raft中有关更改currentTerm，votedFor，log更改的所有位置都需要进行持久化。

2. 分4部分进行。

   1. persist函数
   2. readPersist函数
   3. 函数中有关更改这三个变量的地方添加persist
   4. Make初始化时候readPersist

3. persisit

   ```go
   func (rf *Raft) persist() {
   	// Your code here (2C).
   	// Example:
   	// w := new(bytes.Buffer)
   	// e := gob.NewEncoder(w)
   	// e.Encode(rf.xxx)
   	// e.Encode(rf.yyy)
   	// data := w.Bytes()
   	// rf.persister.SaveRaftState(data)
     //直接模仿即可
   	buf := new(bytes.Buffer)
   	enc := gob.NewEncoder(buf)
   	enc.Encode(rf.currentTerm)
   	enc.Encode(rf.votedFor)
   	enc.Encode(rf.log)
   	rf.persister.SaveRaftState(buf.Bytes())
   }
   ```

4. readPersist

   ```go
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
   ```

   ​

5. 更改上述三量的地方添加persisit. Raft make函数中添加readPersist。

6. 至此完成lab2所有的内容。运行go test -run Test 测试所有的测试例子