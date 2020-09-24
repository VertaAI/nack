package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	jsmapi "github.com/nats-io/jsm.go/api"
	apis "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1"
	typed "github.com/nats-io/nack/pkg/jetstream/generated/clientset/versioned/typed/jetstream/v1"

	k8sapi "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	consumerFinalizerKey = "consumerfinalizer.jetstream.nats.io"
)

func (c *Controller) runConsumerQueue() {
	for {
		processQueueNext(c.cnsQueue, &realJsmClient{}, c.processConsumer)
	}
}

func (c *Controller) processConsumer(ns, name string, jsmc jsmClient) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to process consumer: %w", err)
		}
	}()

	cns, err := c.cnsLister.Consumers(ns).Get(name)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	spec := cns.Spec
	ifc := c.ji.Consumers(ns)

	defer func() {
		if err == nil {
			return
		}

		if _, serr := setConsumerErrored(c.ctx, cns, ifc, err); serr != nil {
			err = fmt.Errorf("%s: %w", err, serr)
		}
	}()

	creds, err := getCreds(c.ctx, spec.CredentialsSecret, c.ki.Secrets(ns))
	if err != nil {
		return err
	}

	c.normalEvent(cns, "Connecting", "Connecting to NATS Server")
	err = jsmc.Connect(
		strings.Join(spec.Servers, ","),
		getNATSOptions(c.natsName, creds)...,
	)
	if err != nil {
		return err
	}
	defer jsmc.Close()
	c.normalEvent(cns, "Connected", "Connected to NATS Server")

	deleteOK := cns.GetDeletionTimestamp() != nil
	newGeneration := cns.Generation != cns.Status.ObservedGeneration
	consumerOK, err := consumerExists(c.ctx, jsmc, spec.StreamName, spec.DurableName)
	if err != nil {
		return err
	}
	updateOK := (consumerOK && !deleteOK && newGeneration)
	createOK := (!consumerOK && !deleteOK && newGeneration)

	switch {
	case createOK:
		c.normalEvent(cns, "Creating",
			fmt.Sprintf("Creating consumer %q on stream %q", spec.DurableName, spec.StreamName))
		if err := createConsumer(c.ctx, jsmc, spec); err != nil {
			return err
		}

		res, err := setConsumerFinalizer(c.ctx, cns, ifc)
		if err != nil {
			return err
		}
		cns = res

		if _, err := setConsumerOK(c.ctx, cns, ifc); err != nil {
			return err
		}
		c.normalEvent(cns, "Created",
			fmt.Sprintf("Created consumer %q on stream %q", spec.DurableName, spec.StreamName))
	case updateOK:
		c.warningEvent(cns, "Updating",
			fmt.Sprintf("Consumer updates (%q on %q) are not allowed, recreate to update", spec.DurableName, spec.StreamName))
	case deleteOK:
		c.normalEvent(cns, "Deleting", fmt.Sprintf("Deleting consumer %q on stream %q", spec.DurableName, spec.StreamName))
		if err := deleteConsumer(c.ctx, jsmc, spec.StreamName, spec.DurableName); err != nil {
			return err
		}

		if _, err := clearConsumerFinalizer(c.ctx, cns, ifc); err != nil {
			return err
		}
	default:
		c.warningEvent(cns, "Noop", fmt.Sprintf("Nothing done for consumer %q", spec.DurableName))
	}

	return nil
}

func consumerExists(ctx context.Context, c jsmClient, stream, consumer string) (ok bool, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to check if consumer exists: %w", err)
		}
	}()

	var apierr jsmapi.ApiError
	_, err = c.LoadConsumer(ctx, stream, consumer)
	if errors.As(err, &apierr) && apierr.NotFoundError() {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func createConsumer(ctx context.Context, c jsmClient, spec apis.ConsumerSpec) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to create consumer %q on stream %q: %w", spec.DurableName, spec.StreamName, err)
		}
	}()

	_, err = c.NewConsumer(ctx, spec.StreamName, jsmapi.ConsumerConfig{
		Durable: spec.DurableName,
	})
	return err
}

func deleteConsumer(ctx context.Context, c jsmClient, stream, consumer string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to delete consumer %q on stream %q: %w", consumer, stream, err)
		}
	}()

	var apierr jsmapi.ApiError
	cn, err := c.LoadConsumer(ctx, stream, consumer)
	if errors.As(err, &apierr) && apierr.NotFoundError() {
		return nil
	} else if err != nil {
		return err
	}

	return cn.Delete()
}

func setConsumerOK(ctx context.Context, s *apis.Consumer, i typed.ConsumerInterface) (*apis.Consumer, error) {
	sc := s.DeepCopy()

	sc.Status.ObservedGeneration = s.Generation
	sc.Status.Conditions = upsertCondition(sc.Status.Conditions, apis.Condition{
		Type:               readyCondType,
		Status:             k8sapi.ConditionTrue,
		LastTransitionTime: time.Now().UTC().Format(time.RFC3339Nano),
		Reason:             "Created",
		Message:            "Consumer successfully created",
	})

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := i.UpdateStatus(ctx, sc, k8smeta.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to set consumer %q status: %w", s.Spec.DurableName, err)
	}

	return res, nil
}

func setConsumerErrored(ctx context.Context, s *apis.Consumer, sif typed.ConsumerInterface, err error) (*apis.Consumer, error) {
	if err == nil {
		return s, nil
	}

	sc := s.DeepCopy()
	sc.Status.Conditions = upsertCondition(sc.Status.Conditions, apis.Condition{
		Type:               readyCondType,
		Status:             k8sapi.ConditionFalse,
		LastTransitionTime: time.Now().UTC().Format(time.RFC3339Nano),
		Reason:             "Errored",
		Message:            err.Error(),
	})

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := sif.UpdateStatus(ctx, sc, k8smeta.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to set consumer errored status: %w", err)
	}

	return res, nil
}


func setConsumerFinalizer(ctx context.Context, s *apis.Consumer, i typed.ConsumerInterface) (*apis.Consumer, error) {
	s.SetFinalizers(addFinalizer(s.GetFinalizers(), streamFinalizerKey))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := i.Update(ctx, s, k8smeta.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to set finalizers for consumer %q: %w", s.Spec.DurableName, err)
	}

	return res, nil
}

func clearConsumerFinalizer(ctx context.Context, s *apis.Consumer, i typed.ConsumerInterface) (*apis.Consumer, error) {
	s.SetFinalizers(removeFinalizer(s.GetFinalizers(), streamFinalizerKey))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := i.Update(ctx, s, k8smeta.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to clear finalizers for consumer %q: %w", s.Spec.DurableName, err)
	}

	return res, nil
}
