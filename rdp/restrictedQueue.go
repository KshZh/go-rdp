package rdp

type node struct {
	msg   *Message
	acked bool
	sent  bool
}

type restrictedQueue struct {
	nodes          []node
	ridx           int
	widx           int
	winSize        int
	maxUnackedMsgs int
}

// 工厂，对客户端屏蔽对象创建细节。
func newRestrictedQueue(initCapacity, winSize, maxUnackedMsgs int) *restrictedQueue {
	return &restrictedQueue{
		nodes:          make([]node, initCapacity),
		ridx:           0,
		widx:           0,
		winSize:        winSize,
		maxUnackedMsgs: maxUnackedMsgs,
	}
}

func (q *restrictedQueue) isEmpty() bool {
	return q.ridx == q.widx
}

// 为了判断队列空还是满，容量为n的队列最多只能插入n-1个元素。
func (q *restrictedQueue) isFull() bool {
	return (q.widx+1)%len(q.nodes) == q.ridx
}

func (q *restrictedQueue) expand() {
	b := make([]node, len(q.nodes)*2)
	i := 0
	for ; q.ridx != q.widx; i++ {
		b[i] = q.nodes[q.ridx] // 注意这里不止要拷贝*Message，而是要拷贝整个node，包括sent和acked这样的字段。
		q.ridx = (q.ridx + 1) % len(q.nodes)
	}
	q.nodes = b
	q.ridx = 0
	q.widx = i
}

func (q *restrictedQueue) push(msg *Message) {
	if q.isFull() {
		q.expand()
	}
	q.nodes[q.widx].msg = msg
	q.nodes[q.widx].acked = false
	q.nodes[q.widx].sent = false
	q.widx = (q.widx + 1) % len(q.nodes)
}

// ack一个已被ack的消息或在窗口外的消息不会有任何副作用。
func (q *restrictedQueue) ack(seqNum int) bool {
	if q.isEmpty() || seqNum < q.nodes[q.ridx].msg.SeqNum {
		return false
	}
	ridx := q.ridx
	flag := false
	for i := 0; i < q.winSize && ridx != q.widx; i++ {
		if q.nodes[ridx].msg.SeqNum == seqNum {
			if !q.nodes[ridx].acked {
				q.nodes[ridx].acked = true
				flag = true // 仅当窗口中的一个已发送且未ack的消息被ack了，才为true。
			}
			break
		}
		ridx = (ridx + 1) % len(q.nodes)
	}
	if !flag {
		return false
	}
	// 注意到，只有窗口的左端点被ack了，窗口才会开始移动。
	ridx = q.ridx
	for i := 0; i < q.winSize && ridx != q.widx && q.nodes[ridx].acked; i++ {
		ridx = (ridx + 1) % len(q.nodes)
	}
	q.ridx = ridx
	return true
}

// 获取窗口中一个可被发送的消息，受到队列大小、窗口大小、maxUnackedMsgs的共同限制，
// 若无法获取这样一个消息，则返回nil。
func (q *restrictedQueue) get() *Message {
	ridx := q.ridx
	cnt := 0
	for i := 0; i < q.winSize && ridx != q.widx && cnt < q.maxUnackedMsgs; i++ {
		if !q.nodes[ridx].sent {
			q.nodes[ridx].sent = true
			return q.nodes[ridx].msg
		} else if !q.nodes[ridx].acked {
			cnt++
		}
		ridx = (ridx + 1) % len(q.nodes)
	}
	return nil
}

func (q *restrictedQueue) debug() (int, int, int, int) {
	ridx := q.ridx
	total, numSent, numAcked := 0, 0, 0
	for i := 0; i < q.winSize && ridx != q.widx; i++ {
		total++
		if q.nodes[ridx].sent {
			numSent++
			if q.nodes[ridx].acked {
				numAcked++
			}
		}
		ridx = (ridx + 1) % len(q.nodes)
	}
	numCanSend := 0
	if numSent-numAcked >= q.maxUnackedMsgs {

	} else if total == numSent {

	} else if q.maxUnackedMsgs-numSent+numAcked < total-numSent {
		numCanSend = q.maxUnackedMsgs - numSent + numAcked
	} else {
		numCanSend = total - numSent
	}
	return total, numSent, numAcked, numCanSend
}
