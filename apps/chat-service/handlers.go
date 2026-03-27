package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"banyan/ledger"
)

type ServiceHandler struct {
	service *ChatService
	mux     *http.ServeMux
}

func NewServiceHandler(svc *ChatService) *ServiceHandler {
	h := &ServiceHandler{
		service: svc,
		mux:     http.NewServeMux(),
	}

	h.mux.HandleFunc("/", h.handleRoot)
	h.mux.HandleFunc("/posts", h.handlePosts)
	h.mux.HandleFunc("/channels", h.handleChannels)
	h.mux.HandleFunc("/events", h.handleWebSocket)

	h.mux.HandleFunc("/ledger/post", h.withKeyCheck("/ledger", h.handleLedgerPost))
	h.mux.HandleFunc("/ledger/delete", h.withKeyCheck("/ledger", h.handleLedgerDelete))
	h.mux.HandleFunc("/ledger/sync", h.withKeyCheck("/ledger", h.handleLedgerSync))

	h.mux.HandleFunc("/mod/ban", h.withKeyCheck("/mod", h.handleModBan))
	h.mux.HandleFunc("/mod/delete", h.withKeyCheck("/mod", h.handleModDelete))
	h.mux.HandleFunc("/mod/promote", h.withKeyCheck("/mod", h.handleModPromote))

	h.mux.HandleFunc("/admin/issue-key", h.withKeyCheck("/admin", h.handleAdminIssueKey))
	h.mux.HandleFunc("/admin/revoke", h.withKeyCheck("/admin", h.handleAdminRevoke))
	h.mux.HandleFunc("/admin/status", h.withKeyCheck("/admin", h.handleAdminStatus))

	return h
}

func (h *ServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *ServiceHandler) withKeyCheck(path string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.service.HasKeyForPath(path) {
			resp, err := h.service.forwarder.Forward(r)
			if err != nil {
				writeError(w, http.StatusBadGateway, "failed to forward request: "+err.Error())
				return
			}
			copyResponse(w, resp)
			return
		}
		handler(w, r)
	}
}

func (h *ServiceHandler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !h.service.config.HasStaticKey {
		writeError(w, http.StatusForbidden, "static key not available")
		return
	}

	if strings.HasPrefix(r.URL.Path, "/assets/") {
		h.serveStatic(w, r)
		return
	}

	if r.URL.Path != "/" {
		if hasFrontend() {
			h.serveIndex(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if hasFrontend() {
		h.serveIndex(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Banyan Chat</h1><p>Webapp not built. Run <code>cd apps/chat-frontend && npm install && npm run build && cp -r dist ../chat-service/frontend/</code></p></body></html>"))
}

func (h *ServiceHandler) serveStatic(w http.ResponseWriter, r *http.Request) {
	fsys := getFrontendFS()
	if fsys == nil {
		writeError(w, http.StatusNotFound, "frontend not available")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css"
	} else if strings.HasSuffix(path, ".html") {
		contentType = "text/html"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(path, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".woff2") {
		contentType = "font/woff2"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func (h *ServiceHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	fsys := getFrontendFS()
	if fsys == nil {
		writeError(w, http.StatusNotFound, "frontend not available")
		return
	}

	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read index.html")
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func (h *ServiceHandler) handlePosts(w http.ResponseWriter, r *http.Request) {
	if !h.service.config.HasStaticKey {
		writeError(w, http.StatusForbidden, "static key not available")
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	state, _ := h.service.ledger.GetState()
	posts := ledger.GetActivePosts(state)

	channelID := r.URL.Query().Get("channel")
	if channelID != "" {
		filtered := make([]*ledger.Post, 0)
		for _, p := range posts {
			if p.ChannelID == channelID {
				filtered = append(filtered, p)
			}
		}
		posts = filtered
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	if offset >= len(posts) {
		posts = []*ledger.Post{}
	} else {
		end := offset + limit
		if end > len(posts) {
			end = len(posts)
		}
		posts = posts[offset:end]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"posts":  posts,
		"count":  len(posts),
		"offset": offset,
		"limit":  limit,
	})
}

func (h *ServiceHandler) handleChannels(w http.ResponseWriter, r *http.Request) {
	if !h.service.config.HasStaticKey {
		writeError(w, http.StatusForbidden, "static key not available")
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	state, _ := h.service.ledger.GetState()
	channels := ledger.GetActiveChannels(state)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels,
		"count":    len(channels),
	})
}

func (h *ServiceHandler) handleLedgerPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		ChannelID string `json:"channelId"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channelId is required")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if len(req.Content) > 10000 {
		writeError(w, http.StatusBadRequest, "content too long (max 10000 chars)")
		return
	}

	state, _ := h.service.ledger.GetState()
	if ledger.IsPeerBanned(state, peerID) {
		writeError(w, http.StatusForbidden, "you are banned")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypePost, peerID, ledger.PostData{
		ChannelID: req.ChannelID,
		Content:   req.Content,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	h.service.BroadcastEvent("post:new", entry)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status": "created",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleLedgerDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		TargetID string `json:"targetId"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "targetId is required")
		return
	}

	targetEntry, err := h.service.ledger.Get(req.TargetID)
	if err != nil || targetEntry == nil {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}

	if targetEntry.Author != peerID {
		writeError(w, http.StatusForbidden, "can only delete own posts")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypeDelete, peerID, ledger.DeleteData{
		TargetID: req.TargetID,
		Reason:   req.Reason,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	h.service.BroadcastEvent("post:delete", map[string]string{"entryId": req.TargetID})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "deleted",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleLedgerSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		since := r.URL.Query().Get("since")
		entries, err := h.service.ledger.GetSince(since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get entries: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entries)

	case http.MethodPost:
		var entries []*ledger.Entry
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.service.ledger.Merge(entries); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to merge entries: "+err.Error())
			return
		}

		h.service.BroadcastEvent("sync:state", map[string]int{"merged": len(entries)})

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "merged",
			"count":  len(entries),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ServiceHandler) handleModBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		TargetPeerID string `json:"targetPeerId"`
		Reason       string `json:"reason"`
		Duration     int64  `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.TargetPeerID == "" {
		writeError(w, http.StatusBadRequest, "targetPeerId is required")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypeBan, peerID, ledger.BanData{
		TargetPeerID: req.TargetPeerID,
		Reason:       req.Reason,
		Duration:     req.Duration,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "banned",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleModDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		TargetID string `json:"targetId"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "targetId is required")
		return
	}

	_, err = h.service.ledger.Get(req.TargetID)
	if err != nil {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypeDelete, peerID, ledger.DeleteData{
		TargetID: req.TargetID,
		Reason:   req.Reason,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	h.service.BroadcastEvent("post:delete", map[string]string{"entryId": req.TargetID})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "deleted",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleModPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		TargetPeerID string `json:"targetPeerId"`
		Role         string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.TargetPeerID == "" {
		writeError(w, http.StatusBadRequest, "targetPeerId is required")
		return
	}
	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypePromote, peerID, ledger.PromoteData{
		TargetPeerID: req.TargetPeerID,
		Role:         req.Role,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "promoted",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleAdminIssueKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		KeyType    string `json:"keyType"`
		TargetNode string `json:"targetNode"`
		ExpiresAt  int64  `json:"expiresAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.KeyType == "" {
		writeError(w, http.StatusBadRequest, "keyType is required")
		return
	}
	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "targetNode is required")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypeIssueKey, peerID, ledger.IssueKeyData{
		KeyType:    req.KeyType,
		TargetNode: req.TargetNode,
		ExpiresAt:  req.ExpiresAt,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "issued",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.service.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	var req struct {
		KeyType    string `json:"keyType"`
		TargetNode string `json:"targetNode"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "targetNode is required")
		return
	}

	prevHash, _ := h.service.ledger.GetLatestHash()
	entry, err := ledger.NewEntry(ledger.EntryTypeRevoke, peerID, ledger.RevokeData{
		KeyType:    req.KeyType,
		TargetNode: req.TargetNode,
		Reason:     req.Reason,
	}, prevHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry: "+err.Error())
		return
	}
	entry.SetHash()

	if err := h.service.ledger.Append(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append entry: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "revoked",
		"entry":  entry,
	})
}

func (h *ServiceHandler) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	state, _ := h.service.ledger.GetState()
	latestHash, _ := h.service.ledger.GetLatestHash()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodeId":       h.service.config.NodeID,
		"latestHash":   latestHash,
		"postCount":    len(state.Posts),
		"channelCount": len(state.Channels),
		"banCount":     len(state.Bans),
		"keys": map[string]bool{
			"static": h.service.config.HasStaticKey,
			"ledger": h.service.config.HasLedgerKey,
			"mod":    h.service.config.HasModKey,
			"admin":  h.service.config.HasAdminKey,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	resp.Body.Close()
}

func hasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}

var _ = hasPrefix

func init() {
	_ = strings.Join(nil, "")
}
