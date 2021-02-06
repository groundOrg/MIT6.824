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
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"../labrpc"
)

// import "bytes"
// import "../labgob"

// some constant
const HEATBEAT float64 = 150   // leader send heatbit per 150ms
const TIMEOUTLOW float64 = 500 // the timeout period randomize between 500ms - 1000ms
const TIMEOUTHIGH float64 = 1000
const CHECKPERIOED float64 = 300 // check timeout per 300ms
const FOLLOWER int = 0
const CANDIDATE int = 1
const LEADER int = 2

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

// log entry struct
type LogEntry struct {
	Term    int
	Command interface{}
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	// Persistent state
	currentTerm int        // latest term server has seen
	votedFor    int        // candidateId that received vote in current term (or null if none)
	log         []LogEntry // first index is 1

	// volatile state on all servers
	commitIndex int       // index of highest log entry known to be committed
	lastApplied int       // index of highest log entry applied to state machine
	timestamp   time.Time // last time receive the leader's heartbeat
	state       int       // follower, candidate, leader

	// volatile state on leaders
	nextIndex []int // for each server, index of next log entry to send to that server
	matchIdex []int // for each server, index of highest log entry known to be replicated on server
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	// Your code here (2A).
	return rf.currentTerm, rf.state == LEADER
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
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int // candidate's term
	CandidateId  int // candidate requsting vote
	LastLogIndex int // index of candidate's last log entry
	LastLogTerm  int // term of candidate's last log entry
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int  // currentTerm, for candidate to update itself
	VoteGranted bool // true means candidate received vote
}

// AppendEntries RPC arguments structure
type AppendEntriesArgs struct {
	Term         int        // leader's term
	LeaderId     int        // so follower canredirect clients
	PrevLogIndex int        // index of log entry immediately preceding new ones
	PrevLogTerm  int        // term of preLogIndex entry
	Entries      []LogEntry // log entries to store (empty for heart beat)
	LeaderCommit int        // leader's commitIndex
}

// AppendEntries RPC reply structure
type AppendEntriesReply struct {
	Term    int  // currentTerm, for leader to update itself
	Success bool // true if follower contained entry matching preLogIndex and prevLogTerm
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// other server has higher term !
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}

	reply.Term = rf.currentTerm
	// reply false immediately if sender's term < currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}

	// to reach this line, the sender must have equal or higher term than me(very likely to be the current leader), reset timer
	rf.timestamp = time.Now()

	// reply false if log doesn't contain an entry at preLogIndex whose term matches preLogTerm
	// remember to handle the case where prevLogIndex points beyond the end of your log
	if args.PrevLogIndex >= len(rf.log) || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Success = false
		return
	}

	// TODO delete and append entries

	reply.Success = true

	if args.LeaderCommit > rf.commitIndex {
		// set commitIndex = min(leaderCommit, index of last **new** entry)
		rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(len(rf.log)-1)))
	}
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	DPrintf("[term %d]: Raft[%d] receive requestVote from Raft[%d]", rf.currentTerm, rf.me, args.CandidateId)

	// reply false immediately if term < currentTerm
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	// other server has higher term !
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}

	reply.Term = rf.currentTerm
	// this server has not voted for other server in this term
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		lastlogterm := rf.log[len(rf.log)-1].Term
		// the candidate's is at least as up-to-date as receiver's log, grant vote !!
		if args.LastLogTerm > lastlogterm ||
			(args.LastLogTerm == lastlogterm && args.LastLogIndex >= len(rf.log)-1) {
			// reset timer only when you **grant** the vote for another server
			rf.timestamp = time.Now()
			rf.votedFor = args.CandidateId
			reply.VoteGranted = true
			DPrintf("[term %d]: Raft [%d] vote for Raft [%d]", rf.currentTerm, rf.me, rf.votedFor)
			return
		}
	}
	reply.VoteGranted = false
	return
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

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	// Your code here (2B).
	index := -1
	term := -1
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// if the Raft instance is not the leader or has been killed, return gracefully
	if rf.state != LEADER || rf.killed() {
		return index, term, false
	}
	// append the entry to Raft's log
	index = len(rf.log)
	term = rf.currentTerm
	rf.log = append(rf.log, LogEntry{Term: rf.currentTerm, Command: command})

	return index, term, true
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// send AppendEntries RPC call to a specific server
// return true if success
// return false otherwise
func (rf *Raft) callAppendEntries(server int, term int, prevLogIndex int, prevLogTerm int, entries []LogEntry, leaderCommit int) bool {
	DPrintf("[term %d]:Raft [%d] [state %d] sends appendentries RPC to server[%d]", rf.currentTerm, rf.me, rf.state, server)
	args := AppendEntriesArgs{
		Term:         term,
		LeaderId:     rf.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: leaderCommit,
	}
	reply := AppendEntriesReply{}
	ok := rf.sendAppendEntries(server, &args, &reply)
	if !ok {
		return false
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// *** to avoid term confusion !!! ***
	// compare the current term with the term you sent in your original RPC.
	// If the two are different, drop the reply and return
	if term != rf.currentTerm {
		return false
	}

	// other server has higher term !
	if reply.Term > rf.currentTerm {
		rf.currentTerm = reply.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}
	return reply.Success

}

// send heartbeat to all other servers (leader only)
func (rf *Raft) leaderHeartBeat() {
	DPrintf("[term %d]:Raft [%d] [state %d] becomes leader !", rf.currentTerm, rf.me, rf.state)
	for {
		rf.mu.Lock()
		// if the server is dead or is not the leader, just return
		if rf.killed() || rf.state != LEADER {
			rf.mu.Unlock()
			return
		}
		term := rf.currentTerm
		leaderCommit := rf.commitIndex
		rf.mu.Unlock()
		for server := range rf.peers {
			if server == rf.me {
				continue
			}
			go func(server int) {
				rf.callAppendEntries(server, term, 0, 0, make([]LogEntry, 0), leaderCommit)
			}(server)
		}
		time.Sleep(time.Millisecond * time.Duration(HEATBEAT))
	}
}

// send RequestVote RPC call to a specific server
// return true if receive vote-granted = true
// return false if not
func (rf *Raft) callRequestVote(server int, term int, lastlogidx int, lastlogterm int) bool {
	DPrintf("[term %d]:Raft [%d][state %d] sends requestvote RPC to server[%d]", term, rf.me, rf.state, server)
	args := RequestVoteArgs{
		Term:         term,
		CandidateId:  rf.me,
		LastLogIndex: lastlogidx,
		LastLogTerm:  lastlogterm,
	}
	reply := RequestVoteReply{}
	ok := rf.sendRequestVote(server, &args, &reply)
	if !ok {
		return false
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	// *** to avoid term confusion !!! ***
	// compare the current term with the term you sent in your original RPC.
	// If the two are different, drop the reply and return
	if term != rf.currentTerm {
		return false
	}

	// other server has higher term !
	if reply.Term > rf.currentTerm {
		rf.currentTerm = reply.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}
	return reply.VoteGranted
}

// candidate starts an election
func (rf *Raft) startElection() {
	rf.mu.Lock()
	rf.currentTerm++          // increment currentTerm
	rf.votedFor = rf.me       // vote for self
	rf.state = CANDIDATE      // convert to candidate
	rf.timestamp = time.Now() // reset election timer
	term := rf.currentTerm    // save for RPC call
	lastlogidx := len(rf.log) - 1
	lastlogterm := rf.log[lastlogidx].Term
	rf.mu.Unlock()
	DPrintf("[term %d]:Raft [%d][state %d] starts an election", term, rf.me, rf.state)
	// send requestVote RPCs to all other servers
	votes := 1                // vote for self
	electionFinished := false // this round of election is finished
	var voteMutex sync.Mutex
	for server := range rf.peers {
		if server == rf.me {
			DPrintf("vote for self : Raft[%d]", rf.me)
			continue
		}
		go func(server int) {
			voteGranted := rf.callRequestVote(server, term, lastlogidx, lastlogterm)
			voteMutex.Lock()
			if voteGranted && !electionFinished {
				votes++
				if votes*2 > len(rf.peers) {
					electionFinished = true
					rf.mu.Lock()
					rf.state = LEADER
					// reinitialize nextIndex and matchIndex after election
					for i := 0; i < len(rf.peers); i++ {
						rf.nextIndex[i] = len(rf.log) // rf.log indexed from 1
						rf.matchIndex[i] = 0
					}
					rf.mu.Unlock()
					go rf.leaderHeartBeat()
				}
			}
			voteMutex.Unlock()
		}(server)
	}
}

// periodically check if it is needed to start an election
func (rf *Raft) electionChecker() {
	r := rand.New(rand.NewSource(int64(rf.me)))
	for {
		// check if dead
		if rf.killed() {
			break
		}
		timeout := int(r.Float64()*(TIMEOUTHIGH-TIMEOUTLOW) + TIMEOUTLOW)
		rf.mu.Lock()
		// if timeout and the server is not a leader, start election
		if time.Since(rf.timestamp) > time.Duration(timeout)*time.Millisecond && rf.state != LEADER {
			// start a new go routine to do the election. This is important
			// so that if you are a candidate (i.e., you are currently running an election),
			// but the election timer fires, you should start another election.
			go rf.startElection()
		}
		rf.mu.Unlock()
		time.Sleep(time.Millisecond * time.Duration(CHECKPERIOED))
	}
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
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.matchIdex = make([]int, len(peers))
	rf.nextIndex = make([]int, len(peers))
	rf.log = make([]LogEntry, 0)
	rf.log = append(rf.log, LogEntry{Term: 0})
	rf.timestamp = time.Now()
	rf.state = FOLLOWER
	rf.votedFor = -1

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start a background goroutine that will kick off leader election periodically
	// by sending out RequestVote RPCs when it hasn't heard from another peer for a while.
	go rf.electionChecker()
	return rf
}
