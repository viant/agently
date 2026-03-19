package v1

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently-core/service/scheduler"
)

type fileScheduleStore struct {
	path string
	mux  sync.Mutex
}

func newFileScheduleStore(workspaceRoot string) *fileScheduleStore {
	base := strings.TrimSpace(workspaceRoot)
	if base == "" {
		base = ".agently"
	}
	return &fileScheduleStore{
		path: filepath.Join(base, "scheduler", "schedules.json"),
	}
}

func (s *fileScheduleStore) Get(id string) (*scheduler.Schedule, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	item, ok := state[id]
	if !ok {
		return nil, nil
	}
	clone := *item
	return &clone, nil
}

func (s *fileScheduleStore) List() ([]*scheduler.Schedule, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	out := make([]*scheduler.Schedule, 0, len(state))
	for _, item := range state {
		clone := *item
		out = append(out, &clone)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *fileScheduleStore) Upsert(item *scheduler.Schedule) error {
	if item == nil {
		return errors.New("schedule is required")
	}
	s.mux.Lock()
	defer s.mux.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	clone := *item
	if strings.TrimSpace(clone.ID) == "" {
		clone.ID = uuid.NewString()
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	if clone.Enabled {
		next := computeNextRunAt(&clone, now)
		if next != nil {
			clone.NextRunAt = next
		}
	} else {
		clone.NextRunAt = nil
	}
	state[clone.ID] = &clone
	return s.saveLocked(state)
}

func (s *fileScheduleStore) Delete(id string) error {
	s.mux.Lock()
	defer s.mux.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	delete(state, strings.TrimSpace(id))
	return s.saveLocked(state)
}

func (s *fileScheduleStore) ListDue(now time.Time) ([]*scheduler.Schedule, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	var out []*scheduler.Schedule
	for _, item := range state {
		if item == nil || !item.Enabled || item.NextRunAt == nil {
			continue
		}
		if !item.NextRunAt.After(now) {
			clone := *item
			out = append(out, &clone)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NextRunAt.Before(*out[j].NextRunAt)
	})
	return out, nil
}

func (s *fileScheduleStore) loadLocked() (map[string]*scheduler.Schedule, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]*scheduler.Schedule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*scheduler.Schedule
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
	}
	state := make(map[string]*scheduler.Schedule, len(list))
	for _, item := range list {
		if item == nil || strings.TrimSpace(item.ID) == "" {
			continue
		}
		clone := *item
		state[clone.ID] = &clone
	}
	return state, nil
}

func (s *fileScheduleStore) saveLocked(state map[string]*scheduler.Schedule) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	list := make([]*scheduler.Schedule, 0, len(state))
	for _, item := range state {
		if item == nil {
			continue
		}
		clone := *item
		list = append(list, &clone)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].UpdatedAt.Equal(list[j].UpdatedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func computeNextRunAt(item *scheduler.Schedule, now time.Time) *time.Time {
	if item == nil || !item.Enabled {
		return nil
	}
	if item.ScheduleType == "interval" && item.IntervalSeconds != nil && *item.IntervalSeconds > 0 {
		next := now.Add(time.Duration(*item.IntervalSeconds) * time.Second)
		return &next
	}
	if item.CronExpr != nil {
		fields := strings.Fields(strings.TrimSpace(*item.CronExpr))
		if len(fields) >= 2 {
			minute, minuteErr := parseIntInRange(fields[0], 0, 59)
			hour, hourErr := parseIntInRange(fields[1], 0, 23)
			if minuteErr == nil && hourErr == nil {
				next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
				if !next.After(now) {
					next = next.Add(24 * time.Hour)
				}
				return &next
			}
		}
	}
	return nil
}

func parseIntInRange(value string, min, max int) (int, error) {
	var parsed int
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			return 0, errors.New("invalid integer")
		}
		parsed = parsed*10 + int(r-'0')
	}
	if parsed < min || parsed > max {
		return 0, errors.New("value out of range")
	}
	return parsed, nil
}
