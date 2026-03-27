package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
)

type EntryType int

const (
	EntryTypePost EntryType = iota
	EntryTypeDelete
	EntryTypeChannelCreate
	EntryTypeChannelDelete
	EntryTypeBan
	EntryTypePromote
	EntryTypeIssueKey
	EntryTypeRevoke
)

func (e EntryType) String() string {
	switch e {
	case EntryTypePost:
		return "post"
	case EntryTypeDelete:
		return "delete"
	case EntryTypeChannelCreate:
		return "channel_create"
	case EntryTypeChannelDelete:
		return "channel_delete"
	case EntryTypeBan:
		return "ban"
	case EntryTypePromote:
		return "promote"
	case EntryTypeIssueKey:
		return "issue_key"
	case EntryTypeRevoke:
		return "revoke"
	default:
		return "unknown"
	}
}

func EntryTypeFromString(s string) (EntryType, error) {
	switch s {
	case "post":
		return EntryTypePost, nil
	case "delete":
		return EntryTypeDelete, nil
	case "channel_create":
		return EntryTypeChannelCreate, nil
	case "channel_delete":
		return EntryTypeChannelDelete, nil
	case "ban":
		return EntryTypeBan, nil
	case "promote":
		return EntryTypePromote, nil
	case "issue_key":
		return EntryTypeIssueKey, nil
	case "revoke":
		return EntryTypeRevoke, nil
	default:
		return EntryTypePost, fmt.Errorf("unknown entry type: %s", s)
	}
}

func (e EntryType) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.String())
}

func (e *EntryType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	et, err := EntryTypeFromString(s)
	if err != nil {
		return err
	}
	*e = et
	return nil
}

type Entry struct {
	ID        string          `json:"id"`
	Type      EntryType       `json:"type"`
	Author    string          `json:"author"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
	Signature []byte          `json:"signature"`
	Hash      string          `json:"hash"`
	PrevHash  string          `json:"prevHash"`
	Deleted   bool            `json:"deleted,omitempty"`
}

type PostData struct {
	ChannelID string `json:"channelId"`
	Content   string `json:"content"`
}

type DeleteData struct {
	TargetID string `json:"targetId"`
	Reason   string `json:"reason"`
}

type ChannelCreateData struct {
	ChannelID   string `json:"channelId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ChannelDeleteData struct {
	ChannelID string `json:"channelId"`
	Reason    string `json:"reason"`
}

type BanData struct {
	TargetPeerID string `json:"targetPeerId"`
	Reason       string `json:"reason"`
	Duration     int64  `json:"duration"`
}

type PromoteData struct {
	TargetPeerID string `json:"targetPeerId"`
	Role         string `json:"role"`
}

type IssueKeyData struct {
	KeyType    string `json:"keyType"`
	TargetNode string `json:"targetNode"`
	ExpiresAt  int64  `json:"expiresAt"`
}

type RevokeData struct {
	KeyType    string `json:"keyType"`
	TargetNode string `json:"targetNode"`
	Reason     string `json:"reason"`
}

func NewEntry(entryType EntryType, author string, data interface{}, prevHash string) (*Entry, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entry data: %w", err)
	}

	return &Entry{
		ID:        uuid.New().String(),
		Type:      entryType,
		Author:    author,
		Timestamp: time.Now().UnixNano(),
		Data:      json.RawMessage(dataBytes),
		PrevHash:  prevHash,
	}, nil
}

func (e *Entry) CalculateHash() string {
	h := sha256.New()
	h.Write([]byte(e.ID))
	h.Write([]byte(e.Type.String()))
	h.Write([]byte(e.Author))
	h.Write([]byte(fmt.Sprintf("%d", e.Timestamp)))
	h.Write(e.Data)
	h.Write([]byte(e.PrevHash))
	return hex.EncodeToString(h.Sum(nil))
}

func (e *Entry) SetHash() {
	e.Hash = e.CalculateHash()
}

func (e *Entry) canonicalBytes() []byte {
	h := sha256.New()
	h.Write([]byte(e.ID))
	h.Write([]byte(e.Type.String()))
	h.Write([]byte(e.Author))
	h.Write([]byte(fmt.Sprintf("%d", e.Timestamp)))
	h.Write(e.Data)
	h.Write([]byte(e.PrevHash))
	return h.Sum(nil)
}

func (e *Entry) Sign(privKey crypto.PrivKey) error {
	sig, err := privKey.Sign(e.canonicalBytes())
	if err != nil {
		return fmt.Errorf("failed to sign entry: %w", err)
	}
	e.Signature = sig
	return nil
}

func (e *Entry) Verify(pubKey crypto.PubKey) error {
	if e.Signature == nil {
		return fmt.Errorf("entry has no signature")
	}
	valid, err := pubKey.Verify(e.canonicalBytes(), e.Signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type Post struct {
	ID        string          `json:"id"`
	Author    string          `json:"author"`
	ChannelID string          `json:"channelId"`
	Content   string          `json:"content"`
	Timestamp int64           `json:"timestamp"`
	Deleted   bool            `json:"deleted"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	CreatedBy   string `json:"createdBy"`
	Deleted     bool   `json:"deleted"`
}

type PeerRole struct {
	PeerID    string `json:"peerId"`
	Role      string `json:"role"`
	GrantedAt int64  `json:"grantedAt"`
	GrantedBy string `json:"grantedBy"`
}

type BanInfo struct {
	PeerID    string `json:"peerId"`
	Reason    string `json:"reason"`
	Duration  int64  `json:"duration"`
	BannedAt  int64  `json:"bannedAt"`
	BannedBy  string `json:"bannedBy"`
	ExpiresAt int64  `json:"expiresAt"`
}

type KeyIssuance struct {
	KeyType    string `json:"keyType"`
	TargetNode string `json:"targetNode"`
	IssuedAt   int64  `json:"issuedAt"`
	IssuedBy   string `json:"issuedBy"`
	ExpiresAt  int64  `json:"expiresAt"`
}

type State struct {
	Posts         map[string]*Post        `json:"posts"`
	Channels      map[string]*Channel     `json:"channels"`
	PeerRoles     map[string]*PeerRole    `json:"peerRoles"`
	Bans          map[string]*BanInfo     `json:"bans"`
	KeyIssuances  map[string]*KeyIssuance `json:"keyIssuances"`
	Revocations   map[string]bool         `json:"revocations"`
	LastEntryHash string                  `json:"lastEntryHash"`
	Entries       []*Entry                `json:"entries"`
}

func NewState() *State {
	return &State{
		Posts:        make(map[string]*Post),
		Channels:     make(map[string]*Channel),
		PeerRoles:    make(map[string]*PeerRole),
		Bans:         make(map[string]*BanInfo),
		KeyIssuances: make(map[string]*KeyIssuance),
		Revocations:  make(map[string]bool),
		Entries:      make([]*Entry, 0),
	}
}

func (s *State) Clone() *State {
	clone := NewState()
	clone.LastEntryHash = s.LastEntryHash

	for k, v := range s.Posts {
		clone.Posts[k] = v
	}
	for k, v := range s.Channels {
		clone.Channels[k] = v
	}
	for k, v := range s.PeerRoles {
		clone.PeerRoles[k] = v
	}
	for k, v := range s.Bans {
		clone.Bans[k] = v
	}
	for k, v := range s.KeyIssuances {
		clone.KeyIssuances[k] = v
	}
	for k, v := range s.Revocations {
		clone.Revocations[k] = v
	}
	clone.Entries = append(clone.Entries, s.Entries...)

	return clone
}

type Ledger interface {
	Append(entry *Entry) error
	Get(id string) (*Entry, error)
	GetSince(hash string) ([]*Entry, error)
	GetState() (*State, error)
	Merge(entries []*Entry) error
	GetLatestHash() (string, error)
	Close() error
}
