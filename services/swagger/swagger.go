package swagger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func (s *SwaggerService) BuildCurl(serverName, operationId string) (string, error) {
	swaggers, err := s.repositories.Swagger.GetServers()
	if err != nil {
		return "", fmt.Errorf("swaggers file: %w", err)
	}
	specPath, ok := swaggers[strings.TrimSpace(serverName)]
	if !ok {
		return "", fmt.Errorf("server %q not found", serverName)
	}
	specPath = strings.TrimSpace(specPath)

	vars, err := s.repositories.Variable.GetVars()
	if err != nil {
		return "", fmt.Errorf("vars file: %w", err)
	}

	data, err := s.repositories.Swagger.LoadSpec(specPath)
	if err != nil {
		return "", err
	}
	spec, err := parseSpec(data)
	if err != nil {
		return "", fmt.Errorf("parse spec: %w", err)
	}

	path, method, op, err := findOperation(spec, operationId)
	if err != nil {
		return "", err
	}

	return buildCurl(spec, path, method, op, vars)
}

func (s *SwaggerService) SaveServerSpec(serverName, specPath string) error {
	return s.repositories.Swagger.SaveServerSpec(serverName, specPath)
}

func (s *SwaggerService) ListServers() ([]string, error) {
	servers, err := s.repositories.Swagger.GetServers()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func parseSpec(data []byte) (*OpenAPI, error) {
	var spec OpenAPI
	if bytes.HasPrefix(bytes.TrimLeft(data, " \t"), []byte("{")) {
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
	}
	return &spec, nil
}

func findOperation(spec *OpenAPI, operationId string) (path string, method string, op *Operation, err error) {
	for p, item := range spec.Paths {
		for _, m := range []struct {
			method string
			op     *Operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"DELETE", item.Delete},
			{"PATCH", item.Patch},
			{"HEAD", item.Head},
			{"OPTIONS", item.Options},
			{"TRACE", item.Trace},
		} {
			if m.op != nil && m.op.OperationID == operationId {
				return p, m.method, m.op, nil
			}
		}
	}
	return "", "", nil, fmt.Errorf("operationId %q not found in spec", operationId)
}

func paramType(p Parameter) string {
	if p.Schema != nil {
		if t, ok := p.Schema["type"].(string); ok {
			return t
		}
	}
	return "string"
}

func propType(propSchema interface{}) string {
	if m, ok := propSchema.(map[string]interface{}); ok {
		if t, ok := m["type"].(string); ok {
			return t
		}
	}
	return "string"
}

func zeroValueForType(typeStr string) string {
	switch typeStr {
	case "integer", "number":
		return "0"
	case "boolean":
		return "false"
	case "array":
		return "[]"
	case "object":
		return "{}"
	}
	return ""
}

func zeroValueForJSON(typeStr string) interface{} {
	switch typeStr {
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		return []interface{}{}
	case "object":
		return map[string]interface{}{}
	}
	return ""
}

func buildCurl(spec *OpenAPI, path, method string, op *Operation, vars map[string]string) (string, error) {
	baseURL := ""
	if len(spec.Servers) > 0 {
		baseURL = strings.TrimSuffix(spec.Servers[0].URL, "/")
	}
	fullPath := path
	pathParams := make(map[string]string)
	queryParams := make(map[string]string)
	headerParams := make(map[string]string)
	for _, p := range op.Parameters {
		val := vars[p.Name]
		if val == "" {
			val = zeroValueForType(paramType(p))
		}
		switch p.In {
		case "path":
			pathParams[p.Name] = val
		case "query":
			queryParams[p.Name] = val
		case "header":
			headerParams[p.Name] = val
		}
	}
	for k, v := range pathParams {
		fullPath = strings.ReplaceAll(fullPath, "{"+k+"}", v)
	}
	url := baseURL + fullPath
	if len(queryParams) > 0 {
		var q []string
		for k, v := range queryParams {
			q = append(q, fmt.Sprintf("%s=%s", k, v))
		}
		url += "?" + strings.Join(q, "&")
	}

	var b strings.Builder
	b.WriteString("curl -X ")
	b.WriteString(method)
	b.WriteString(" ")
	for k, v := range headerParams {
		b.WriteString(fmt.Sprintf("-H %q ", k+": "+v))
	}
	if op.RequestBody != nil && len(op.RequestBody.Content) > 0 {
		if ct, ok := op.RequestBody.Content["application/json"]; ok && ct.Schema != nil {
			body := make(map[string]interface{})
			if props, ok := ct.Schema["properties"].(map[string]interface{}); ok {
				for k, propSchema := range props {
					if v, has := vars[k]; has {
						body[k] = v
					} else {
						body[k] = zeroValueForJSON(propType(propSchema))
					}
				}
			}
			if len(body) > 0 {
				raw, _ := json.Marshal(body)
				b.WriteString(fmt.Sprintf("-H %q ", "Content-Type: application/json"))
				b.WriteString(fmt.Sprintf("-d %q ", string(raw)))
			}
		}
	}
	b.WriteString(url)
	return b.String(), nil
}
