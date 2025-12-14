package queue

type Queue[T any] struct {
	data []T
}

func New[T any]() *Queue[T] {
	return &Queue[T]{data: make([]T, 0)}
}

func (q *Queue[T]) Push(item T) {
	q.data = append(q.data, item)
}

func (q *Queue[T]) Pop() (T, bool) {
	if len(q.data) == 0 {
		var zero T
		return zero, false
	}
	item := q.data[0]
	q.data = q.data[1:]
	return item, true
}

func (q *Queue[T]) MustPop() T {
	item, ok := q.Pop()
	if !ok {
		panic("pop from empty queue")
	}
	return item
}

func (q *Queue[T]) Get(i int) (T, bool) {
	if i < 0 || i >= len(q.data) {
		var zero T
		return zero, false
	}
	return q.data[i], true
}

func (q *Queue[T]) Len() int {
	return len(q.data)
}
