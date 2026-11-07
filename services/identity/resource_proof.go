package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type ResourceProof struct {
	SessionID     string `json:"sessionID"`
	UserID        string `json:"userID"`
	Epoch         int64  `json:"epoch"`
	Platform      int    `json:"platform"`
	LogicalClient string `json:"logicalClient"`
	Origin        string `json:"origin"`
	ExpiresAt     int64  `json:"expiresAt"`
}

func (s *Store) IssueResourceProof(ctx context.Context, session Session, origin string) (string, error) {
	v, err := s.Live(ctx, session.SessionID)
	if err != nil || v.UserID != session.UserID || v.Epoch != session.Epoch || v.Platform != session.Platform || origin == "" {
		return "", ErrSession
	}
	client, err := SessionClient(session)
	if err != nil || client != v.LogicalClient {
		return "", ErrSession
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random[:])
	proof := ResourceProof{SessionID: v.SessionID, UserID: v.UserID, Epoch: v.Epoch, Platform: v.Platform, LogicalClient: client, Origin: origin, ExpiresAt: v.ExpiresAt}
	if err = s.Write(ctx, "HRM38:RESOURCE_PROOF:"+Hash(token), proof, time.Duration(v.ExpiresAt-s.now().Unix())*time.Second); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) ValidateResourceProof(ctx context.Context, token, origin string) (Session, error) {
	var proof ResourceProof
	if len(token) != 64 || origin == "" {
		return Session{}, ErrSession
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || hex.EncodeToString(decoded) != token || s.Read(ctx, "HRM38:RESOURCE_PROOF:"+Hash(token), &proof) != nil || proof.Origin != origin || proof.ExpiresAt <= s.now().Unix() {
		return Session{}, ErrSession
	}
	v, err := s.Live(ctx, proof.SessionID)
	if err != nil || v.UserID != proof.UserID || v.Epoch != proof.Epoch || v.Platform != proof.Platform || v.LogicalClient != proof.LogicalClient {
		return Session{}, ErrSession
	}
	return v, nil
}
