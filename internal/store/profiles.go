package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/storagebench/storagebench/internal/benchmark"
)

// ProfileStep is one test within a profile. It is a full benchmark spec; when a
// profile is fired against hosts, the same step spec is used for every selected
// host (the target path/kind live in the spec). Steps run in array order.
type ProfileStep struct {
	Spec benchmark.Spec `json:"spec"`
}

// Profile is a named, ordered collection of test steps.
type Profile struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Steps       []ProfileStep `json:"steps"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

// Validate checks the profile and every step spec against the benchmark
// allowlists. A profile that passes is safe to execute.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("profile must have at least one step")
	}
	for i := range p.Steps {
		s := p.Steps[i].Spec
		if err := s.Validate(); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, s.Name, err)
		}
	}
	return nil
}

// CreateProfile inserts a new profile and returns it with id/timestamps set.
func (s *Store) CreateProfile(p Profile) (Profile, error) {
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	p.ID = newID("prof")
	now := time.Now().UTC().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now
	steps, _ := json.Marshal(p.Steps)

	_, err := s.db.Exec(`
		INSERT INTO test_profile (id, name, description, steps, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, string(steps), p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return Profile{}, err
	}
	return p, nil
}

// UpdateProfile overwrites name/description/steps of an existing profile.
func (s *Store) UpdateProfile(p Profile) (Profile, error) {
	if p.ID == "" {
		return Profile{}, fmt.Errorf("profile id required")
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	steps, _ := json.Marshal(p.Steps)
	res, err := s.db.Exec(`
		UPDATE test_profile SET name=?, description=?, steps=?, updated_at=?
		WHERE id=?`,
		p.Name, p.Description, string(steps), p.UpdatedAt, p.ID)
	if err != nil {
		return Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, fmt.Errorf("profile %q not found", p.ID)
	}
	// Preserve created_at by reading it back.
	_ = s.db.QueryRow(`SELECT created_at FROM test_profile WHERE id=?`, p.ID).Scan(&p.CreatedAt)
	return p, nil
}

// DeleteProfile removes a profile. Runs that referenced it keep their frozen
// config (the FK is ON DELETE SET NULL).
func (s *Store) DeleteProfile(id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`DELETE FROM test_profile WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("profile %q not found", id)
	}
	return nil
}

// GetProfile returns one profile, ok=false if not found.
func (s *Store) GetProfile(id string) (Profile, bool, error) {
	var p Profile
	var steps string
	err := s.db.QueryRow(`
		SELECT id, name, description, steps, created_at, updated_at
		FROM test_profile WHERE id=?`, id).Scan(
		&p.ID, &p.Name, &p.Description, &steps, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	_ = json.Unmarshal([]byte(steps), &p.Steps)
	return p, true, nil
}

// ListProfiles returns all profiles, newest first.
func (s *Store) ListProfiles() ([]Profile, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, steps, created_at, updated_at
		FROM test_profile ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var steps string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &steps, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(steps), &p.Steps)
		out = append(out, p)
	}
	return out, rows.Err()
}
