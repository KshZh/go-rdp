package rdp

import (
	"container/heap"
	"time"
)

// 问题：心跳需不需要超时重传？
// 并不需要，因为超时重传至少隔了一个epoch，如果本端在这个epoch中还是没有发送任何数据包，就会再发一个心跳。

// 连接建立后往堆中放入两个事件，一个heartbeat，一个lost，每次发送了数据包，就重置heartbeat事件，每次收到数据包就重置lost事件。
// 每次发送一个数据包就往堆中放入数据包对应的trace事件，若收到了对该数据包的ack，则从堆中删去该trace事件，否则超时重传，
// 按照指数退避的策略重置该trace事件的过期时间。

// 每次重置堆中的事件，需要重置计时器，确保以堆顶的事件，即最早过期的事件倒计时。

// 大写字母开头的会被export，虽然没有export的必要，
// 但小写字母开头作为类型名觉得别扭。
type eventType int

const (
	heartbeat eventType = iota // 一段时间没有发送数据包，发一个心跳告诉对端自己还活着，重置对端的lost事件的过期时间。
	trace                      // 跟踪数据包，超时重传。
	lost                       // 超过一段时间没有收到对端的数据包，认定连接断开/丢失。
)

const (
	heartbeatSeqNum = 0
	lostSeqNum      = -1
)

type event struct {
	t           eventType
	msg         *Message
	currBackOff int // 类似TCP的拥塞控制，当发生超时时，协议假设网络拥塞，主动指数退避(2^n)，避免一直注入流量，使网络一直拥塞。
	expiration  time.Time
}

type key struct {
	connID int
	seqNum int
}

type eventHeap struct {
	// 在堆内存上创建一个实体，然后用指针指向它，
	// 传递指针，这样修改和访问的始终是同一个对象，且减少内存拷贝，
	// 如果传递副本，则就不是修改和访问同一个对象了。
	// 这种做法对只读对象有明显的好处，即可以让多个指针指向它，无需拷贝多份。
	// 对于可写对象，如果多个位置的代码想要修改同一个对象的话，也要用这种做法。
	events []*event
	idx    map[key]int // (connID, seqNum) -> event在堆中的下标。
}

func (eh eventHeap) Len() int { return len(eh.events) }

func (eh eventHeap) Less(i, j int) bool {
	return eh.events[i].expiration.Before(eh.events[j].expiration)
}

func (eh eventHeap) Swap(i, j int) {
	// 别忘了交换下标，从而正确反映元素在堆中的位置。
	eh.idx[key{eh.events[i].msg.ConnID, eh.events[i].msg.SeqNum}] = j
	eh.idx[key{eh.events[j].msg.ConnID, eh.events[j].msg.SeqNum}] = i
	eh.events[i], eh.events[j] = eh.events[j], eh.events[i]
}

func (eh *eventHeap) Push(x interface{}) {
	item := x.(*event)
	eh.idx[key{item.msg.ConnID, item.msg.SeqNum}] = len(eh.events)
	eh.events = append(eh.events, item)
}

// 看src/container/heap/heap.go，heap.Pop()和heap.Remove()都会回调我们实现的Pop()，
// 后者先把要删除的元素交换到数组最后，调整堆序后，调用我们实现的Pop()。
func (eh *eventHeap) Pop() interface{} {
	n := len(eh.events)
	item := eh.events[n-1]
	eh.events[n-1] = nil // avoid memory leak.
	delete(eh.idx, key{item.msg.ConnID, item.msg.SeqNum})
	eh.events = eh.events[0 : n-1]
	return item
}

// 接口：

// 对客户端屏蔽对象创建细节。
func newEventHeap() *eventHeap {
	return &eventHeap{idx: make(map[key]int)}
}

func (eh *eventHeap) push(x *event) {
	heap.Push(eh, x)
}

func (eh *eventHeap) pop() *event {
	return heap.Pop(eh).(*event)
}

func (eh *eventHeap) updateExpiration(connID, seqNum int, expiration time.Time) {
	if i, ok := eh.idx[key{connID, seqNum}]; ok {
		eh.events[i].expiration = expiration
		heap.Fix(eh, i) // 调整堆序。
	} else {
		panic("")
	}
}

func (eh *eventHeap) remove(connID, seqNum int) {
	// 如果删除的元素不存在，报出来，有助于定位bug。
	if _, ok := eh.idx[key{connID, seqNum}]; !ok {
		panic("")
	}
	heap.Remove(eh, eh.idx[key{connID, seqNum}])
	// delete(eh.idx, Key{connID, seqNum}) // 不需要，因为已经在我们实现的Pop()中删除了。
}

func (eh *eventHeap) removeAll(connID int) {
	for k, v := range eh.idx {
		if k.connID == connID {
			heap.Remove(eh, v)
		}
	}
}

func (eh *eventHeap) get(connID, seqNum int) *event {
	i, ok := eh.idx[key{connID, seqNum}]
	if !ok {
		return nil
	}
	return eh.events[i]
}

func (eh *eventHeap) top() *event {
	if len(eh.events) > 0 {
		return eh.events[0]
	}
	return nil
}
