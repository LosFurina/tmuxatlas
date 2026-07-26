package webpush

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

const subscriptionsFileName = "push-subscriptions.json"

type subscriptionFile struct {
	Version       int                `json:"version"`
	Subscriptions []*wp.Subscription `json:"subscriptions"`
}

// Store manages durable push notification subscriptions, keyed by endpoint.
type Store struct {
	mu            sync.RWMutex
	path          string
	subscriptions map[string]*wp.Subscription
	write         func(string, map[string]*wp.Subscription) error
}

func NewStore() (*Store, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStoreAt(filepath.Join(dir, subscriptionsFileName))
}

func NewStoreAt(path string) (*Store, error) {
	store := &Store{
		path: path, subscriptions: make(map[string]*wp.Subscription),
		write: writeSubscriptions,
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read push subscriptions: %w", err)
	}
	var file subscriptionFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("decode push subscriptions: %w", err)
	}
	if file.Version != 1 {
		return fmt.Errorf("unsupported push subscription store version %d", file.Version)
	}
	for _, subscription := range file.Subscriptions {
		if subscription == nil || subscription.Endpoint == "" {
			return errors.New("push subscription store contains an invalid endpoint")
		}
		s.subscriptions[subscription.Endpoint] = cloneSubscription(subscription)
	}
	return nil
}

func (s *Store) Add(subscription *wp.Subscription) error {
	if subscription == nil || subscription.Endpoint == "" {
		return errors.New("push subscription endpoint is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneSubscriptions(s.subscriptions)
	next[subscription.Endpoint] = cloneSubscription(subscription)
	if err := s.write(s.path, next); err != nil {
		return err
	}
	s.subscriptions = next
	return nil
}

func (s *Store) Remove(endpoint string) error {
	if endpoint == "" {
		return errors.New("push subscription endpoint is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscriptions[endpoint]; !ok {
		return nil
	}
	next := cloneSubscriptions(s.subscriptions)
	delete(next, endpoint)
	if err := s.write(s.path, next); err != nil {
		return err
	}
	s.subscriptions = next
	return nil
}

func (s *Store) All() []*wp.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	endpoints := make([]string, 0, len(s.subscriptions))
	for endpoint := range s.subscriptions {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	result := make([]*wp.Subscription, 0, len(endpoints))
	for _, endpoint := range endpoints {
		result = append(result, cloneSubscription(s.subscriptions[endpoint]))
	}
	return result
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscriptions)
}

func writeSubscriptions(path string, subscriptions map[string]*wp.Subscription) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create push subscription directory: %w", err)
	}
	file := subscriptionFile{Version: 1}
	endpoints := make([]string, 0, len(subscriptions))
	for endpoint := range subscriptions {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	for _, endpoint := range endpoints {
		file.Subscriptions = append(file.Subscriptions, cloneSubscription(subscriptions[endpoint]))
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal push subscriptions: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".push-subscriptions-*")
	if err != nil {
		return fmt.Errorf("create push subscription temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace push subscription store: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err == nil {
		defer directory.Close()
		if syncErr := directory.Sync(); syncErr != nil {
			return syncErr
		}
	}
	return nil
}

func cloneSubscriptions(source map[string]*wp.Subscription) map[string]*wp.Subscription {
	target := make(map[string]*wp.Subscription, len(source))
	for endpoint, subscription := range source {
		target[endpoint] = cloneSubscription(subscription)
	}
	return target
}

func cloneSubscription(source *wp.Subscription) *wp.Subscription {
	if source == nil {
		return nil
	}
	target := *source
	return &target
}
