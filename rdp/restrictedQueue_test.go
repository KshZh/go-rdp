package rdp

import "testing"

func TestRestrictedQueueBasic(t *testing.T) {
	q := newRestrictedQueue(DefaultContainerSize, 0, 0)
	msgs := make([]Message, 40)
	i := 0
	for ; i < 39; i++ {
		q.push(&msgs[i])
	}
	// 初始容量为20，若插入39个*Message，那么容量为40，
	// 若再插入一个*Message，则此时会重新分配内存空间，容量为80。
	if len(q.nodes) != 40 {
		t.Error("")
	}
	q.push(&msgs[i])
	i++
	if len(q.nodes) != 80 {
		t.Error("")
	}
}

func TestRestrictedQueueGetAck(t *testing.T) {
	initCapacity, winSize, maxUnackedMsgs, numMsgs := 10, 5, 3, 20
	q := newRestrictedQueue(initCapacity, winSize, maxUnackedMsgs)
	msgs := make([]Message, numMsgs)
	for i := 0; i < numMsgs; i++ {
		msgs[i].SeqNum = i
	}

	q.push(&msgs[0])
	q.push(&msgs[1])
	if msg := q.get(); msg == nil || msg.SeqNum != 0 {
		t.Error("")
	}
	if msg := q.get(); msg == nil || msg.SeqNum != 1 {
		t.Error("")
	}
	t.Log("1. 受到队列大小的限制。")
	a, b, c, d := q.debug()
	t.Logf("total: %d, numSent: %d, numAcked: %d, numCanSend: %d", a, b, c, d)
	if msg := q.get(); msg != nil {
		t.Error("")
	}
	// 解除队列大小的限制。
	q.push(&msgs[2])
	q.push(&msgs[3])

	if msg := q.get(); msg == nil || msg.SeqNum != 2 {
		t.Error("")
	}
	t.Log("2. 受到maxUnackedMsgs的限制。")
	a, b, c, d = q.debug()
	t.Logf("total: %d, numSent: %d, numAcked: %d, numCanSend: %d", a, b, c, d)
	if msg := q.get(); msg != nil {
		t.Error("")
	}
	// 解除maxUnackedMsgs的限制。
	if !q.ack(1) {
		t.Error("")
	}
	if msg := q.get(); msg == nil || msg.SeqNum != 3 {
		t.Error("")
	}

	if !q.ack(2) || !q.ack(3) {
		t.Error("")
	}
	q.push(&msgs[4])
	q.push(&msgs[5])
	if msg := q.get(); msg == nil || msg.SeqNum != 4 {
		t.Error("")
	}
	t.Log("3. 受到窗口大小的限制。")
	a, b, c, d = q.debug()
	t.Logf("total: %d, numSent: %d, numAcked: %d, numCanSend: %d", a, b, c, d)
	if msg := q.get(); msg != nil {
		t.Error("")
	}
	// 解除窗口大小的限制。
	// 注意到，只有窗口的左端点被ack了，窗口才会开始移动。
	if !q.ack(0) {
		t.Error("")
	}
	if msg := q.get(); msg == nil || msg.SeqNum != 5 {
		t.Error("")
	}
	if !q.ack(5) || !q.ack(4) {
		t.Error("")
	}
	if !q.isEmpty() {
		t.Error("")
	}

	q.push(&msgs[6])
	q.push(&msgs[7])
	if msg := q.get(); msg == nil || msg.SeqNum != 6 {
		t.Error("")
	}
	if msg := q.get(); msg == nil || msg.SeqNum != 7 {
		t.Error("")
	}
	if msg := q.get(); msg != nil {
		t.Error("")
	}
	if !q.ack(6) || !q.ack(7) {
		t.Error("")
	}
	if !q.isEmpty() {
		t.Error("")
	}

	// ack窗口外的消息返回false，且没有副作用。
	if q.ack(8) || q.ack(1) {
		t.Error("")
	}
}
