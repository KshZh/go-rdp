package rdp

import "testing"

func TestQueue(t *testing.T) {
	q := newQueue(DefaultContainerSize)
	for i := 0; i < DefaultContainerSize-1; i++ {
		q.push(i)
	}
	for i := 0; i < DefaultContainerSize-1; i++ {
		if q.pop().(int) != i {
			t.Error("")
		}
	}
	if q.pop() != nil {
		t.Error("")
	}

	// 测试一下expand。
	for i := 0; i < DefaultContainerSize-1; i++ {
		q.push(i)
	}
	if len(q.nodes) != DefaultContainerSize {
		t.Error("")
	}
	q.push(DefaultContainerSize - 1)
	if len(q.nodes) != DefaultContainerSize*2 {
		t.Error("")
	}
	for i := 0; i < DefaultContainerSize; i++ {
		if q.pop().(int) != i {
			t.Error("")
		}
	}
	if q.pop() != nil {
		t.Error("")
	}
}
