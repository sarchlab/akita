package codec

import (
	"reflect"
	"sync"
	"testing"
)

func TestRegistryConcurrentRegistration(t *testing.T) {
	r := NewRegistry[any]("test")
	if r.Contains(nil) || r.Contains(square{}) || len(r.Tags()) != 0 {
		t.Fatal("new registry is not empty")
	}
	r.Register(square{})

	// Different array lengths produce distinct types and tags. Multiple writers
	// must retain all additions, not overwrite one another's snapshots.
	values := make([]any, 32)
	for i := range values {
		values[i] = reflect.Zero(reflect.ArrayOf(i+1, reflect.TypeFor[byte]())).Interface()
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, v := range values {
		workers.Go(func() {
			<-start
			for range 100 {
				r.Register(v)
				if !r.Contains(v) {
					t.Errorf("registration of %T was not published", v)
					return
				}
			}
		})
	}
	for range 8 {
		workers.Go(func() {
			<-start
			checkConcurrentRegistryReads(t, r, values)
		})
	}
	close(start)
	workers.Wait()

	for _, v := range values {
		if !r.Contains(v) {
			t.Errorf("lost registration of %T", v)
		}
		if err := r.CheckRoundTrip(v); err != nil {
			t.Error(err)
		}
	}
	if got := len(r.Tags()); got != len(values)+1 {
		t.Fatalf("got %d tags, want %d unique types", got, len(values)+1)
	}
	t.Logf("%d distinct types remain registered and decodable after concurrent registration and reads", len(r.Tags()))
}

func checkConcurrentRegistryReads(t *testing.T, r *Registry[any], values []any) {
	t.Helper()
	for i := range 500 {
		if !r.Contains(square{}) {
			t.Error("previously registered type disappeared")
			return
		}
		if len(r.Tags()) == 0 {
			t.Error("registered tags disappeared")
			return
		}
		v := values[i%len(values)]
		if !r.Contains(v) {
			continue // Its writer may not have registered it yet.
		}
		encoded, err := r.Encode(v)
		if err != nil {
			t.Error(err)
			return
		}
		decoded, err := r.Decode(encoded)
		if err != nil || !reflect.DeepEqual(decoded, v) {
			t.Errorf("published %T failed decode: value=%v, err=%v", v, decoded, err)
			return
		}
	}
}
