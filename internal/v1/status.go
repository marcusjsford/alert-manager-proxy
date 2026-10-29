package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/upstream"

	amconfig "github.com/prometheus/alertmanager/config"
	"gopkg.in/yaml.v2"
)

type v1ClusterStatus struct {
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Peers  []v1PeerStatus `json:"peers"`
}

type v1PeerStatus struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type v2Status struct {
	Cluster *v1ClusterStatus `json:"cluster"`
	Config  struct {
		Original string `json:"original"`
	} `json:"config"`
	Uptime      time.Time         `json:"uptime"`
	VersionInfo map[string]string `json:"versionInfo"`
}

type v1Status struct {
	ConfigYAML    string            `json:"configYAML"`
	ConfigJSON    map[string]any    `json:"configJSON"`
	VersionInfo   map[string]string `json:"versionInfo"`
	Uptime        time.Time         `json:"uptime"`
	ClusterStatus *v1ClusterStatus  `json:"clusterStatus"`
}

func (r *Router) getStatus(w http.ResponseWriter, req *http.Request) {
	v2Req, err := r.upstream.Request(req.Context(), http.MethodGet, "/api/v2/status", nil, nil)
	if err != nil {
		upstream.WriteFailure(w, req, err, r.logger)
		return
	}
	upstream.ForwardHeaders(v2Req.Header, req.Header)
	var status v2Status
	if err := r.upstream.DoJSON(v2Req, &status); err != nil {
		r.writeAlertsUpstreamError(w, req, err)
		return
	}

	configYAML, configJSON, stripped, err := renderLegacyConfig(status.Config.Original)
	if err != nil {
		envelope.Error(w, http.StatusInternalServerError, "internal", "failed to render upstream config")
		return
	}
	if stripped && r.obs != nil {
		r.obs.Lossy(req.Context(), "status.config", "target-version-only fields omitted")
	}
	envelope.SuccessData(w, v1Status{
		ConfigYAML:    configYAML,
		ConfigJSON:    configJSON,
		VersionInfo:   status.VersionInfo,
		Uptime:        status.Uptime,
		ClusterStatus: status.Cluster,
	})
}

// renderLegacyConfig turns the target's rendered YAML into the v1 shape. The
// target adds fields that v0.26 status consumers never saw; omit those while
// keeping the same YAML renderer (yaml.v2) and the target's secret masking.
func renderLegacyConfig(original string) (string, map[string]any, bool, error) {
	cfg, err := amconfig.Load(original)
	if err != nil {
		return "", nil, false, err
	}
	configYAML, yamlRemoved, err := legacyConfigYAML(cfg.String())
	if err != nil {
		return "", nil, false, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", nil, false, err
	}
	var configJSON map[string]any
	if err := json.Unmarshal(raw, &configJSON); err != nil {
		return "", nil, false, err
	}
	jsonRemoved := stripLegacyConfigJSON(configJSON)
	return configYAML, configJSON, yamlRemoved || jsonRemoved, nil
}

var legacyGlobalFields = []string{"smtp_tls_config", "slack_app_url", "rocketchat_api_url"}

func legacyConfigYAML(raw string) (string, bool, error) {
	var document yaml.MapSlice
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return "", false, err
	}
	removed := removeMapSlice(&document, "event_recorder", "tracing")
	if global, ok := mapSliceValue(document, "global").(yaml.MapSlice); ok {
		removed = removeMapSlice(&global, legacyGlobalFields...) || removed
		setMapSliceValue(&document, "global", global)
	}
	if receivers, ok := mapSliceValue(document, "receivers").([]interface{}); ok {
		for i, item := range receivers {
			receiver, ok := item.(yaml.MapSlice)
			if !ok {
				continue
			}
			removed = removeMapSlice(&receiver, "labels") || removed
			for j := range receiver {
				key := fmt.Sprint(receiver[j].Key)
				if !strings.HasSuffix(key, "_configs") {
					continue
				}
				configs, ok := receiver[j].Value.([]interface{})
				if !ok {
					continue
				}
				for k, configItem := range configs {
					configMap, ok := configItem.(yaml.MapSlice)
					if !ok {
						continue
					}
					removed = removeMapSlice(&configMap, "timeout") || removed
					configs[k] = configMap
				}
				receiver[j].Value = configs
			}
			receivers[i] = receiver
		}
		setMapSliceValue(&document, "receivers", receivers)
	}
	out, err := yaml.Marshal(document)
	return string(out), removed, err
}

func mapSliceValue(document yaml.MapSlice, key string) any {
	for _, item := range document {
		if fmt.Sprint(item.Key) == key {
			return item.Value
		}
	}
	return nil
}

func setMapSliceValue(document *yaml.MapSlice, key string, value any) {
	for i := range *document {
		if fmt.Sprint((*document)[i].Key) == key {
			(*document)[i].Value = value
			return
		}
	}
}

func removeMapSlice(document *yaml.MapSlice, keys ...string) bool {
	remove := make(map[string]bool, len(keys))
	for _, key := range keys {
		remove[key] = true
	}
	out := (*document)[:0]
	removed := false
	for _, item := range *document {
		if remove[fmt.Sprint(item.Key)] {
			removed = true
			continue
		}
		out = append(out, item)
	}
	*document = out
	return removed
}

func stripLegacyConfigJSON(configJSON map[string]any) bool {
	removed := false
	for _, key := range []string{"event_recorder", "tracing"} {
		if _, ok := configJSON[key]; ok {
			delete(configJSON, key)
			removed = true
		}
	}
	if templates, ok := configJSON["templates"].([]any); ok && len(templates) == 0 {
		// v0.26's JSON rendering exposed null for an empty template list,
		// while the target marshals an empty slice.
		configJSON["templates"] = nil
	}
	if global, ok := configJSON["global"].(map[string]any); ok {
		for _, key := range legacyGlobalFields {
			if _, ok := global[key]; ok {
				delete(global, key)
				removed = true
			}
		}
	}
	receivers, ok := configJSON["receivers"].([]any)
	if !ok {
		return removed
	}
	for _, item := range receivers {
		receiver, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := receiver["labels"]; ok {
			delete(receiver, "labels")
			removed = true
		}
		for key, value := range receiver {
			if !strings.HasSuffix(key, "_configs") {
				continue
			}
			configs, ok := value.([]any)
			if !ok {
				continue
			}
			for _, configItem := range configs {
				configMap, ok := configItem.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := configMap["timeout"]; ok {
					delete(configMap, "timeout")
					removed = true
				}
			}
		}
	}
	return removed
}
