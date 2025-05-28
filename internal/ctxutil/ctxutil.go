package ctxutil

import "context"

func Recv[T any, C ~<-chan T](ctx context.Context, c C) (v T, ok bool) {
	select {
	case <-ctx.Done():
		return v, false
	case v, ok := <-c:
		return v, ok
	}
}
