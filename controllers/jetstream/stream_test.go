package jetstream

import (
	"context"
	"errors"
	"strings"
	"testing"

	jsmapi "github.com/nats-io/jsm.go/api"

	apis "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1"
	clientsetfake "github.com/nats-io/nack/pkg/jetstream/generated/clientset/versioned/fake"

	k8sapis "k8s.io/api/core/v1"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sclientsetfake "k8s.io/client-go/kubernetes/fake"
	k8stypedfake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
)

func TestProcessStream(t *testing.T) {
	t.Parallel()

	updateObject := func(a k8stesting.Action) (handled bool, o runtime.Object, err error) {
		ua, ok := a.(k8stesting.UpdateAction)
		if !ok {
			return false, nil, nil
		}

		return true, ua.GetObject(), nil
	}

	t.Run("create stream", func(t *testing.T) {
		t.Parallel()

		jc := clientsetfake.NewSimpleClientset()
		wantEvents := 4
		rec := record.NewFakeRecorder(wantEvents)
		ctrl := NewController(Options{
			Ctx:            context.Background(),
			KubeIface:      k8sclientsetfake.NewSimpleClientset(),
			JetstreamIface: jc,
			Recorder:       rec,
		})

		ns, name := "default", "my-stream"

		informer := ctrl.informerFactory.Jetstream().V1().Streams()
		err := informer.Informer().GetStore().Add(&apis.Stream{
			ObjectMeta: k8smeta.ObjectMeta{
				Namespace:  ns,
				Name:       name,
				Generation: 1,
			},
			Spec: apis.StreamSpec{
				Name:    name,
				MaxAge:  "1h",
				Storage: "memory",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		jc.PrependReactor("update", "streams", updateObject)

		notFoundErr := jsmapi.ApiError{Code: 404}
		jsmc := &mockJsmClient{
			loadStreamErr: notFoundErr,
		}
		if err := ctrl.processStream(ns, name, jsmc); err != nil {
			t.Fatal(err)
		}

		if got := len(rec.Events); got != wantEvents {
			t.Error("unexpected number of events")
			t.Fatalf("got=%d; want=%d", got, wantEvents)
		}

		<-rec.Events
		<-rec.Events
		for i := 0; i < len(rec.Events); i++ {
			gotEvent := <-rec.Events
			if !strings.Contains(gotEvent, "Creat") {
				t.Error("unexpected event")
				t.Fatalf("got=%s; want=%s", gotEvent, "Creating/Created...")
			}
		}
	})

	t.Run("create stream with credentials", func(t *testing.T) {
		t.Parallel()

		const secretName, secretKey = "mysecret", "nats-creds"
		getSecret := func(a k8stesting.Action) (handled bool, o runtime.Object, err error) {
			ga, ok := a.(k8stesting.GetAction)
			if !ok {
				return false, nil, nil
			}
			if ga.GetName() != secretName {
				return false, nil, nil
			}

			return true, &k8sapis.Secret{
				Data: map[string][]byte{
					secretKey: []byte("... creds..."),
				},
			}, nil
		}

		jc := clientsetfake.NewSimpleClientset()
		kc := k8sclientsetfake.NewSimpleClientset()
		wantEvents := 4
		rec := record.NewFakeRecorder(wantEvents)
		ctrl := NewController(Options{
			Ctx:            context.Background(),
			KubeIface:      kc,
			JetstreamIface: jc,
			Recorder:       rec,
		})

		ns, name := "default", "my-stream"

		informer := ctrl.informerFactory.Jetstream().V1().Streams()
		err := informer.Informer().GetStore().Add(&apis.Stream{
			ObjectMeta: k8smeta.ObjectMeta{
				Namespace:  ns,
				Name:       name,
				Generation: 1,
			},
			Spec: apis.StreamSpec{
				Name:    name,
				MaxAge:  "1h",
				Storage: "memory",
				CredentialsSecret: apis.CredentialsSecret{
					Name: secretName,
					Key:  "nats-creds",
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		jc.PrependReactor("update", "streams", updateObject)
		kc.CoreV1().(*k8stypedfake.FakeCoreV1).PrependReactor("get", "secrets", getSecret)

		notFoundErr := jsmapi.ApiError{Code: 404}
		jsmc := &mockJsmClient{
			loadStreamErr: notFoundErr,
		}
		if err := ctrl.processStream(ns, name, jsmc); err != nil {
			t.Fatal(err)
		}

		if got := len(rec.Events); got != wantEvents {
			t.Error("unexpected number of events")
			t.Fatalf("got=%d; want=%d", got, wantEvents)
		}

		<-rec.Events
		<-rec.Events
		for i := 0; i < len(rec.Events); i++ {
			gotEvent := <-rec.Events
			if !strings.Contains(gotEvent, "Creat") {
				t.Error("unexpected event")
				t.Fatalf("got=%s; want=%s", gotEvent, "Creating/Created...")
			}
		}
	})

	t.Run("update stream", func(t *testing.T) {
		t.Parallel()

		jc := clientsetfake.NewSimpleClientset()
		wantEvents := 4
		rec := record.NewFakeRecorder(wantEvents)
		ctrl := NewController(Options{
			Ctx:            context.Background(),
			KubeIface:      k8sclientsetfake.NewSimpleClientset(),
			JetstreamIface: jc,
			Recorder:       rec,
		})

		ns, name := "default", "my-stream"

		informer := ctrl.informerFactory.Jetstream().V1().Streams()
		err := informer.Informer().GetStore().Add(&apis.Stream{
			ObjectMeta: k8smeta.ObjectMeta{
				Namespace:  ns,
				Name:       name,
				Generation: 2,
			},
			Spec: apis.StreamSpec{
				Name:    name,
				MaxAge:  "1h",
				Storage: "memory",
			},
			Status: apis.Status{
				ObservedGeneration: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		jc.PrependReactor("update", "streams", updateObject)

		jsmc := &mockJsmClient{
			loadStreamErr: nil,
			loadStream:    &mockStream{},
		}
		if err := ctrl.processStream(ns, name, jsmc); err != nil {
			t.Fatal(err)
		}

		if got := len(rec.Events); got != wantEvents {
			t.Error("unexpected number of events")
			t.Fatalf("got=%d; want=%d", got, wantEvents)
		}

		<-rec.Events
		<-rec.Events
		for i := 0; i < len(rec.Events); i++ {
			gotEvent := <-rec.Events
			if !strings.Contains(gotEvent, "Updat") {
				t.Error("unexpected event")
				t.Fatalf("got=%s; want=%s", gotEvent, "Updating/Updated...")
			}
		}
	})

	t.Run("delete stream", func(t *testing.T) {
		t.Parallel()

		jc := clientsetfake.NewSimpleClientset()
		wantEvents := 3
		rec := record.NewFakeRecorder(wantEvents)
		ctrl := NewController(Options{
			Ctx:            context.Background(),
			KubeIface:      k8sclientsetfake.NewSimpleClientset(),
			JetstreamIface: jc,
			Recorder:       rec,
		})

		ts := k8smeta.Unix(1600216923, 0)
		ns, name := "default", "my-stream"

		informer := ctrl.informerFactory.Jetstream().V1().Streams()
		err := informer.Informer().GetStore().Add(&apis.Stream{
			ObjectMeta: k8smeta.ObjectMeta{
				Namespace:         ns,
				Name:              name,
				Finalizers:        []string{streamFinalizerKey},
				Generation:        2,
				DeletionTimestamp: &ts,
			},
			Spec: apis.StreamSpec{
				Name:    name,
				MaxAge:  "1h",
				Storage: "memory",
			},
			Status: apis.Status{
				ObservedGeneration: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		jc.PrependReactor("update", "streams", updateObject)

		jsmc := &mockJsmClient{
			loadStreamErr: nil,
			loadStream:    &mockStream{},
		}
		if err := ctrl.processStream(ns, name, jsmc); err != nil {
			t.Fatal(err)
		}

		if got := len(rec.Events); got != wantEvents {
			t.Error("unexpected number of events")
			t.Fatalf("got=%d; want=%d", got, wantEvents)
		}

		<-rec.Events
		<-rec.Events
		for i := 0; i < len(rec.Events); i++ {
			gotEvent := <-rec.Events
			if !strings.Contains(gotEvent, "Delet") {
				t.Error("unexpected event")
				t.Fatalf("got=%s; want=%s", gotEvent, "Deleting/Deleted...")
			}
		}
	})

	t.Run("process error", func(t *testing.T) {
		t.Parallel()

		jc := clientsetfake.NewSimpleClientset()
		wantEvents := 4
		rec := record.NewFakeRecorder(wantEvents)
		ctrl := NewController(Options{
			Ctx:            context.Background(),
			KubeIface:      k8sclientsetfake.NewSimpleClientset(),
			JetstreamIface: jc,
			Recorder:       rec,
		})

		ns, name := "default", "my-stream"

		informer := ctrl.informerFactory.Jetstream().V1().Streams()
		err := informer.Informer().GetStore().Add(&apis.Stream{
			ObjectMeta: k8smeta.ObjectMeta{
				Namespace:  ns,
				Name:       name,
				Generation: 1,
			},
			Spec: apis.StreamSpec{
				Name:    name,
				MaxAge:  "1h",
				Storage: "memory",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		jc.PrependReactor("update", "streams", func(a k8stesting.Action) (handled bool, o runtime.Object, err error) {
			ua, ok := a.(k8stesting.UpdateAction)
			if !ok {
				return false, nil, nil
			}
			obj := ua.GetObject()

			str, ok := obj.(*apis.Stream)
			if !ok {
				t.Error("unexpected object type")
				t.Fatalf("got=%T; want=%T", obj, &apis.Stream{})
			}

			if got, want := len(str.Status.Conditions), 1; got != want {
				t.Error("unexpected number of conditions")
				t.Fatalf("got=%d; want=%d", got, want)
			}
			if got, want := str.Status.Conditions[0].Reason, "Errored"; got != want {
				t.Error("unexpected condition reason")
				t.Fatalf("got=%s; want=%s", got, want)
			}

			return true, obj, nil
		})

		jsmc := &mockJsmClient{
			connectErr: errors.New("nats connect failed"),
		}
		if err := ctrl.processStream(ns, name, jsmc); err == nil {
			t.Fatal("unexpected success")
		}
	})
}
