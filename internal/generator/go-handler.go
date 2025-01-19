package generator

import (
	"fmt"
	"master-gen/internal/parser"
	"master-gen/internal/util"
	"regexp"
	"strings"
)

func generateHandler(op parser.Operation) string {
	var goCode string
	switch op.Type {
	case "row":
		goCode = generateQueryRowFunc(op)
	case "rows":
		goCode = generateQueryRowsFunc(op)
	case "execute":
		goCode = generateQueryExecFunc(op)
	default:
		if op.Handler == "" {
			return ""
		}
		goCode = generateCustomFunc(op)
	}

	goCode += `
	h.JSON.Success(w, res)
}	
`
	return goCode
}

func generateQueryRowFunc(op parser.Operation) string {
	goCode := ""
	goCode += `
func (h *Handler) ` + op.Method + op.Name + `(w http.ResponseWriter, r *http.Request) {`
	if op.QueryParams != nil {
		goCode += generateQueryParams(op.Name, op.QueryParams)
	}
	if op.Body != nil {
		goCode += generateBody(op.Name, op.Body)
	}
	goCode += generateQueryRow(op.Name, op.Query, op.QueryParams, op.Body, op.Res)

	return goCode
}

func generateQueryRowsFunc(op parser.Operation) string {
	goCode := ""
	goCode += `
func (h *Handler) ` + op.Method + op.Name + `(w http.ResponseWriter, r *http.Request) {`
	if op.QueryParams != nil {
		goCode += generateQueryParams(op.Name, op.QueryParams)
	}
	if op.Body != nil {
		goCode += generateBody(op.Name, op.Body)
	}
	goCode += generateQueryRows(op.Name, op.Query, op.QueryParams, op.Res)

	return goCode
}

func generateQueryExecFunc(op parser.Operation) string {
	goCode := ""
	goCode += `
func (h *Handler) ` + op.Method + op.Name + `(w http.ResponseWriter, r *http.Request) {`
	if op.QueryParams != nil {
		goCode += generateQueryParams(op.Name, op.QueryParams)
	}
	if op.Body != nil {
		goCode += generateBody(op.Name, op.Body)
	}
	query, _ := processQuery(op.Query)
	goCode += `
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := "` + query + `"

	`

	inserts := make([]string, 0)
	if len(op.QueryParams) > 0 {
		for k := range op.QueryParams {
			inserts = append(inserts, "queryParams."+util.Capitalize(k))
		}
	}
	if len(op.Body) > 0 {
		for k := range op.Body {
			inserts = append(inserts, "body."+util.Capitalize(k))
		}
	}

	goCode += `
	_, err := h.DB.ExecContext(ctx, query, ` + strings.Join(inserts, ", ") + `)
	if err != nil {
		h.JSON.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	res := map[string]string{"message": "success"}
`

	return goCode
}

func generateCustomFunc(op parser.Operation) string {
	goCode := ""
	goCode += `
func (h *Handler) ` + op.Method + op.Name + `(w http.ResponseWriter, r *http.Request) {`
	if op.QueryParams != nil {
		goCode += generateQueryParams(op.Name, op.QueryParams)
	}
	if op.Body != nil {
		goCode += generateBody(op.Name, op.Body)
	}
	goCode += generateCustom(op.Handler, op.QueryParams)

	return goCode
}

func generateQueryParams(name string, params parser.QueryParams) string {
	if params == nil {
		return ""
	}

	goCode := `
	queryParams := types.` + name + `Query{`
	for k := range params {
		goCode += `
		` + util.Capitalize(k) + `: r.URL.Query().Get("` + k + `"),`
	}
	goCode += `
	}
	
				`
	return goCode
}

func generateBody(name string, body parser.Body) string {
	if body == nil {
		return ""
	}

	goCode := `
	body := types.` + name + `Body{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.JSON.Error(w, http.StatusBadRequest, "Failed to decode body")
		return
	}
	`
	return goCode
}

func generateQueryRow(name, queryStr string, params parser.QueryParams, body parser.Body, res parser.Response) string {

	inserts := make([]string, 0)
	if len(params) > 0 {
		for k := range params {
			inserts = append(inserts, "queryParams."+util.Capitalize(k))
		}
	}
	if len(body) > 0 {
		for k := range body {
			inserts = append(inserts, "body."+util.Capitalize(k))
		}
	}

	query, _ := processQuery(queryStr)

	goCode := `
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := "` + query + `"

	`

	scan := make([]string, 0)
	for k := range res {
		scan = append(scan, "&res."+util.Capitalize(k))
	}
	goCode += `
	res := types.` + name + `Row{}
	err := h.DB.QueryRowContext(ctx, query, ` + strings.Join(inserts, ", ") + `)`
	if len(scan) > 0 {
		goCode += `.Scan(` + strings.Join(scan, ", ") + `)`
	}
	goCode += `
	if err != nil {
		h.JSON.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
`

	return goCode
}

func generateQueryRows(name, queryStr string, params parser.QueryParams, res parser.Response) string {

	inserts := make([]string, 0)
	for k := range params {
		inserts = append(inserts, "queryParams."+util.ConvertToCamelCase(k))
	}

	scan := make([]string, 0)
	for k := range res {

		scan = append(scan, "&row."+util.ConvertToCamelCase(k))
	}

	query, _ := processQuery(queryStr)

	goCode := `
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := "` + query + `"

	rows, err := h.DB.QueryContext(ctx, query, ` + strings.Join(inserts, ", ") + `)
	if err != nil {
		h.JSON.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	res := make([]types.` + name + `Row, 0)
	for rows.Next() {
		row := types.` + name + `Row{}
		err = rows.Scan(` + strings.Join(scan, ", ") + `)
		if err != nil {
			h.JSON.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		res = append(res, row)
	}
`

	return goCode
}

func generateCustom(handler string, params parser.QueryParams) string {
	return `
	res, err := injection.` + processHandler(handler, params) + `
	if err != nil {
		h.JSON.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	`
}

func processQuery(query string) (string, map[string]string) {
	// Regex patterns
	extrapolateRegex := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}\$`) // Matches `{XXX}$`
	insertRegex := regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)      // Matches `${XXX}`

	// Step 1: Replace {XXX}$ → XXX
	query = extrapolateRegex.ReplaceAllString(query, "$1")

	// Step 2: Replace ${XXX} with $1, $2, etc., and track numbers
	placeholderMap := make(map[string]string)
	counter := 1

	query = insertRegex.ReplaceAllStringFunc(query, func(match string) string {
		// Extract key inside ${XXX}
		key := match[2 : len(match)-1] // Removes ${ and }
		if _, exists := placeholderMap[key]; !exists {
			placeholderMap[key] = fmt.Sprintf("$%d", counter)
			counter++
		}
		return placeholderMap[key]
	})

	return query, placeholderMap
}

func processHandler(handler string, queryParams map[string]string) string {
	// Replace `${}` with `w, r`
	handler = regexp.MustCompile(`\$\{\}`).ReplaceAllString(handler, "w, r")

	// Replace `${example}` (or any query param key) with its value
	paramRegex := regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)
	handler = paramRegex.ReplaceAllStringFunc(handler, func(match string) string {
		key := match[2 : len(match)-1] // Extracts key inside `${example}`
		if val, exists := queryParams[key]; exists {
			return val
		}
		return match // Keep unchanged if no matching param found
	})

	return handler
}
