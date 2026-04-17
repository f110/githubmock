package githubmock

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/graph-gophers/graphql-go"
)

func (m *Mock) registerGraphQLService(mux *http.ServeMux) {
	schema := graphql.MustParseSchema(graphqlSchema, &rootResolver{mock: m})

	m.registerHandleFunc(mux, "POST /graphql", func(w http.ResponseWriter, req *http.Request) {
		var params struct {
			Query         string         `json:"query"`
			OperationName string         `json:"operationName"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(req.Body).Decode(&params); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"message": err.Error()}}})
			return
		}

		resp := schema.Exec(req.Context(), params.Query, params.OperationName, params.Variables)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			m.Logger.ErrorContext(req.Context(), "failed to encode graphql response", slog.Any("err", err))
		}
	})
}
