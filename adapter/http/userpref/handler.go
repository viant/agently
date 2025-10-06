package userpref

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	authctx "github.com/viant/agently/internal/auth"
	convsvc "github.com/viant/agently/internal/service/conversation"
	usersvc "github.com/viant/agently/internal/service/user"
)

type apiResponse struct {
	Status  string `json:"status"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

func Handler() (http.Handler, error) {
	dao, err := convsvc.NewDatly(context.Background())
	if err != nil {
		return nil, err
	}
	us, err := usersvc.New(context.Background(), dao)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/me/preferences", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		uname := strings.TrimSpace(authctx.EffectiveUserID(r.Context()))
		if uname == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: "unauthorized"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			v, err := us.FindByUsername(r.Context(), uname)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
				return
			}
			if v == nil {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: "user not found"})
				return
			}
			data := map[string]any{
				"username":           v.Username,
				"displayName":        value(v.DisplayName),
				"timezone":           v.Timezone,
				"defaultAgentRef":    value(v.DefaultAgentRef),
				"defaultModelRef":    value(v.DefaultModelRef),
				"defaultEmbedderRef": value(v.DefaultEmbedderRef),
			}
			_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: data})
		case http.MethodPatch:
			var p struct {
				DisplayName        *string `json:"displayName"`
				Timezone           *string `json:"timezone"`
				DefaultAgentRef    *string `json:"defaultAgentRef"`
				DefaultModelRef    *string `json:"defaultModelRef"`
				DefaultEmbedderRef *string `json:"defaultEmbedderRef"`
			}
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
				return
			}
			if err := us.UpdatePreferencesByUsername(r.Context(), uname, p.DisplayName, p.Timezone, p.DefaultAgentRef, p.DefaultModelRef, p.DefaultEmbedderRef); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: "updated"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux, nil
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}
