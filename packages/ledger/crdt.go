package ledger

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
)

type ConflictInfo struct {
	EntryID    string `json:"entryId"`
	EntryType  string `json:"entryType"`
	Author1    string `json:"author1"`
	Author2    string `json:"author2"`
	Timestamp1 int64  `json:"timestamp1"`
	Timestamp2 int64  `json:"timestamp2"`
	Winner     string `json:"winner"`
	Reason     string `json:"reason"`
}

func CompareEntries(e1, e2 *Entry) int {
	if e1.Timestamp != e2.Timestamp {
		if e1.Timestamp > e2.Timestamp {
			return 1
		}
		return -1
	}
	if e1.Author > e2.Author {
		return 1
	} else if e1.Author < e2.Author {
		return -1
	}
	return 0
}

func SortEntries(entries []*Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return CompareEntries(entries[i], entries[j]) < 0
	})
}

func Merge(existing, incoming []*Entry) ([]*Entry, []ConflictInfo) {
	entryMap := make(map[string]*Entry)
	conflicts := make([]ConflictInfo, 0)

	for _, e := range existing {
		entryMap[e.ID] = e
	}

	for _, e := range incoming {
		existingEntry, exists := entryMap[e.ID]
		if !exists {
			entryMap[e.ID] = e
			continue
		}

		cmp := CompareEntries(e, existingEntry)
		if cmp > 0 {
			conflicts = append(conflicts, ConflictInfo{
				EntryID:    e.ID,
				EntryType:  e.Type.String(),
				Author1:    existingEntry.Author,
				Author2:    e.Author,
				Timestamp1: existingEntry.Timestamp,
				Timestamp2: e.Timestamp,
				Winner:     e.Author,
				Reason:     "later_timestamp",
			})
			entryMap[e.ID] = e
		} else if cmp == 0 && e.Author != existingEntry.Author {
			conflicts = append(conflicts, ConflictInfo{
				EntryID:    e.ID,
				EntryType:  e.Type.String(),
				Author1:    existingEntry.Author,
				Author2:    e.Author,
				Timestamp1: existingEntry.Timestamp,
				Timestamp2: e.Timestamp,
				Winner:     existingEntry.Author,
				Reason:     "same_timestamp_peer_id_tiebreaker",
			})
		}
	}

	result := make([]*Entry, 0, len(entryMap))
	for _, e := range entryMap {
		result = append(result, e)
	}

	SortEntries(result)

	return result, conflicts
}

func MergeIntoState(state *State, entries []*Entry) ([]ConflictInfo, error) {
	conflicts := make([]ConflictInfo, 0)

	for _, entry := range entries {
		alreadyProcessed := false
		for _, existing := range state.Entries {
			if existing.ID == entry.ID {
				alreadyProcessed = true
				break
			}
		}
		if alreadyProcessed {
			continue
		}

		var err error
		switch entry.Type {
		case EntryTypePost:
			err = applyPost(state, entry)
		case EntryTypeDelete:
			err = applyDelete(state, entry, &conflicts)
		case EntryTypeChannelCreate:
			err = applyChannelCreate(state, entry)
		case EntryTypeChannelDelete:
			err = applyChannelDelete(state, entry, &conflicts)
		case EntryTypeBan:
			err = applyBan(state, entry)
		case EntryTypePromote:
			err = applyPromote(state, entry)
		case EntryTypeIssueKey:
			err = applyIssueKey(state, entry)
		case EntryTypeRevoke:
			err = applyRevoke(state, entry)
		}

		if err != nil {
			log.Printf("Error applying entry %s: %v", entry.ID, err)
			continue
		}

		state.Entries = append(state.Entries, entry)
		state.LastEntryHash = entry.Hash
	}

	return conflicts, nil
}

func applyPost(state *State, entry *Entry) error {
	var data PostData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal post data: %w", err)
	}

	state.Posts[entry.ID] = &Post{
		ID:        entry.ID,
		Author:    entry.Author,
		ChannelID: data.ChannelID,
		Content:   data.Content,
		Timestamp: entry.Timestamp,
		Deleted:   false,
	}
	return nil
}

func applyDelete(state *State, entry *Entry, conflicts *[]ConflictInfo) error {
	var data DeleteData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal delete data: %w", err)
	}

	post, exists := state.Posts[data.TargetID]
	if !exists {
		return fmt.Errorf("post %s not found", data.TargetID)
	}

	if post.Deleted {
		return nil
	}

	post.Deleted = true

	for _, e := range state.Entries {
		if e.ID == data.TargetID {
			e.Deleted = true
			break
		}
	}

	return nil
}

func applyChannelCreate(state *State, entry *Entry) error {
	var data ChannelCreateData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal channel create data: %w", err)
	}

	state.Channels[data.ChannelID] = &Channel{
		ID:          data.ChannelID,
		Name:        data.Name,
		Description: data.Description,
		CreatedAt:   entry.Timestamp,
		CreatedBy:   entry.Author,
		Deleted:     false,
	}
	return nil
}

func applyChannelDelete(state *State, entry *Entry, conflicts *[]ConflictInfo) error {
	var data ChannelDeleteData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal channel delete data: %w", err)
	}

	channel, exists := state.Channels[data.ChannelID]
	if !exists {
		return fmt.Errorf("channel %s not found", data.ChannelID)
	}

	channel.Deleted = true
	return nil
}

func applyBan(state *State, entry *Entry) error {
	var data BanData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal ban data: %w", err)
	}

	expiresAt := int64(0)
	if data.Duration > 0 {
		expiresAt = entry.Timestamp + data.Duration
	}

	state.Bans[data.TargetPeerID] = &BanInfo{
		PeerID:    data.TargetPeerID,
		Reason:    data.Reason,
		Duration:  data.Duration,
		BannedAt:  entry.Timestamp,
		BannedBy:  entry.Author,
		ExpiresAt: expiresAt,
	}
	return nil
}

func applyPromote(state *State, entry *Entry) error {
	var data PromoteData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal promote data: %w", err)
	}

	state.PeerRoles[data.TargetPeerID] = &PeerRole{
		PeerID:    data.TargetPeerID,
		Role:      data.Role,
		GrantedAt: entry.Timestamp,
		GrantedBy: entry.Author,
	}
	return nil
}

func applyIssueKey(state *State, entry *Entry) error {
	var data IssueKeyData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal issue key data: %w", err)
	}

	key := fmt.Sprintf("%s:%s", data.KeyType, data.TargetNode)
	state.KeyIssuances[key] = &KeyIssuance{
		KeyType:    data.KeyType,
		TargetNode: data.TargetNode,
		IssuedAt:   entry.Timestamp,
		IssuedBy:   entry.Author,
		ExpiresAt:  data.ExpiresAt,
	}
	return nil
}

func applyRevoke(state *State, entry *Entry) error {
	var data RevokeData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Errorf("failed to unmarshal revoke data: %w", err)
	}

	key := fmt.Sprintf("%s:%s", data.KeyType, data.TargetNode)
	state.Revocations[key] = true
	return nil
}

func IsTombstoned(state *State, entryID string) bool {
	for _, entry := range state.Entries {
		if entry.Type == EntryTypeDelete {
			var data DeleteData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				if data.TargetID == entryID {
					return true
				}
			}
		}
	}
	return false
}

func GetActivePosts(state *State) []*Post {
	posts := make([]*Post, 0)
	for _, post := range state.Posts {
		if !post.Deleted {
			posts = append(posts, post)
		}
	}
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Timestamp < posts[j].Timestamp
	})
	return posts
}

func GetActiveChannels(state *State) []*Channel {
	channels := make([]*Channel, 0)
	for _, channel := range state.Channels {
		if !channel.Deleted {
			channels = append(channels, channel)
		}
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].CreatedAt < channels[j].CreatedAt
	})
	return channels
}

func IsPeerBanned(state *State, peerID string) bool {
	ban, exists := state.Bans[peerID]
	if !exists {
		return false
	}
	if ban.Duration == 0 {
		return true
	}
	return ban.ExpiresAt > 0
}

type CRDTMerger struct {
	state   *State
	entries map[string]*Entry
	hashIdx map[string]*Entry
	heads   map[string]bool
}

func NewCRDTMerger() *CRDTMerger {
	return &CRDTMerger{
		state:   NewState(),
		entries: make(map[string]*Entry),
		hashIdx: make(map[string]*Entry),
		heads:   make(map[string]bool),
	}
}

func (m *CRDTMerger) AddEntry(entry *Entry) error {
	if existing, exists := m.entries[entry.ID]; exists {
		if CompareEntries(entry, existing) <= 0 {
			return nil
		}
	}

	m.entries[entry.ID] = entry
	m.hashIdx[entry.Hash] = entry
	m.heads[entry.Hash] = true
	delete(m.heads, entry.PrevHash)

	return nil
}

func (m *CRDTMerger) GetEntry(id string) *Entry {
	return m.entries[id]
}

func (m *CRDTMerger) GetEntryByHash(hash string) *Entry {
	return m.hashIdx[hash]
}

func (m *CRDTMerger) GetAllEntries() []*Entry {
	result := make([]*Entry, 0, len(m.entries))
	for _, entry := range m.entries {
		result = append(result, entry)
	}
	SortEntries(result)
	return result
}

func (m *CRDTMerger) GetLatestHash() string {
	for hash := range m.heads {
		return hash
	}
	return ""
}

func (m *CRDTMerger) ComputeState() *State {
	m.state = NewState()
	entries := m.GetAllEntries()
	MergeIntoState(m.state, entries)
	return m.state
}
