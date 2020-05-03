package rdp

// Under the hood, interface values can be thought of as a tuple of a value and a concrete type: (value, type)。
// （也就是指向内存块的指针和类型标识符，后者指明了如何解释这块内存）
// An empty interface may hold values of any type. (Every type implements at least zero methods.)

type queue struct {
	nodes []interface{}
	ridx  int
	widx  int
}

func newQueue(initCapacity int) *queue {
	return &queue{nodes: make([]interface{}, initCapacity)}
}

func (q *queue) isEmpty() bool {
	return q.ridx == q.widx
}

// 为了判断队列空还是满，容量为n的队列最多只能插入n-1个元素。
func (q *queue) isFull() bool {
	return (q.widx+1)%len(q.nodes) == q.ridx
}

func (q *queue) expand() {
	b := make([]interface{}, len(q.nodes)*2)
	i := 0
	for ; q.ridx != q.widx; i++ {
		b[i] = q.nodes[q.ridx]
		q.ridx = (q.ridx + 1) % len(q.nodes)
	}
	q.nodes = b
	q.ridx = 0
	q.widx = i
}

func (q *queue) push(x interface{}) {
	if q.isFull() {
		q.expand()
	}
	q.nodes[q.widx] = x
	q.widx = (q.widx + 1) % len(q.nodes)
}

func (q *queue) pop() interface{} {
	if q.isEmpty() {
		return nil
	}
	x := q.nodes[q.ridx]
	q.ridx = (q.ridx + 1) % len(q.nodes)
	return x
}
