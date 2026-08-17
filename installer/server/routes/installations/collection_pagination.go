package installations

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/RobertWHurst/navaros"
)

const (
	defaultInstallationLimit = 100
	maxInstallationLimit     = 100
	installationPageKey      = "installations.collectionPage"
)

type installationSort struct {
	field string
	desc  bool
}

type installationPage struct {
	limit  int
	offset int
	sorts  []installationSort
}

func installationPaginationMiddleware(ctx *navaros.Context) {
	page, err := parseInstallationPage(ctx.Request().URL)
	if err != nil {
		ctx.Status = http.StatusBadRequest
		ctx.Body = map[string]string{"error": err.Error()}
		return
	}
	ctx.Set(installationPageKey, page)
	ctx.Next()
}

func parseInstallationPage(requestURL *url.URL) (installationPage, error) {
	page := installationPage{limit: defaultInstallationLimit}
	query := requestURL.Query()
	for key, target := range map[string]*int{"$limit": &page.limit, "$offset": &page.offset} {
		raw := strings.TrimSpace(query.Get(key))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return page, fmt.Errorf("%s must be a non-negative integer", key)
		}
		*target = value
	}
	if page.limit > maxInstallationLimit {
		page.limit = maxInstallationLimit
	}

	for _, component := range strings.Split(requestURL.RawQuery, "&") {
		if component == "" {
			continue
		}
		parts := strings.SplitN(component, "=", 2)
		key, err := url.QueryUnescape(parts[0])
		if err != nil {
			continue
		}
		value := ""
		if len(parts) == 2 {
			value, _ = url.QueryUnescape(parts[1])
		}
		if field, ok := installationOperatorField(key, "$sort"); ok {
			if !installationSortableField(field) {
				continue
			}
			direction := strings.ToLower(strings.TrimSpace(value))
			if direction != "asc" && direction != "desc" {
				continue
			}
			page.sorts = append(page.sorts, installationSort{field: field, desc: direction == "desc"})
			continue
		}
	}
	if len(page.sorts) == 0 {
		page.sorts = []installationSort{{field: "startedAt"}}
	}
	if !installationSortContains(page.sorts, "id") {
		page.sorts = append(page.sorts, installationSort{field: "id"})
	}
	return page, nil
}

func installationOperatorField(key, operator string) (string, bool) {
	prefix := operator + "["
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "]") {
		return "", false
	}
	field := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "]")
	if field == "" || strings.ContainsAny(field, "_-.[") {
		return "", false
	}
	return field, true
}

func installationSortableField(field string) bool {
	switch field {
	case "id", "payload", "disk", "status", "progress", "startedAt":
		return true
	default:
		return false
	}
}

func installationSortContains(fields []installationSort, field string) bool {
	for _, candidate := range fields {
		if candidate.field == field {
			return true
		}
	}
	return false
}

func paginateInstallations(page installationPage, installations []*Installation) ([]*Installation, error) {
	sort.SliceStable(installations, func(i, j int) bool {
		return compareInstallations(installations[i], installations[j], page.sorts) < 0
	})

	start := page.offset
	if start > len(installations) {
		start = len(installations)
	}
	if page.limit == 0 {
		return []*Installation{}, nil
	}
	end := start + page.limit
	if end > len(installations) {
		end = len(installations)
	}
	return installations[start:end], nil
}

func compareInstallations(left, right *Installation, fields []installationSort) int {
	for _, field := range fields {
		comparison := compareInstallationField(left, right, field.field)
		if comparison == 0 {
			continue
		}
		if field.desc {
			return -comparison
		}
		return comparison
	}
	return 0
}

func compareInstallationField(left, right *Installation, field string) int {
	switch field {
	case "id":
		leftID, leftErr := strconv.Atoi(left.ID)
		rightID, rightErr := strconv.Atoi(right.ID)
		if leftErr == nil && rightErr == nil {
			return compareOrdered(leftID, rightID)
		}
		return strings.Compare(left.ID, right.ID)
	case "payload":
		return strings.Compare(left.Payload, right.Payload)
	case "disk":
		return strings.Compare(left.Disk, right.Disk)
	case "status":
		return strings.Compare(left.Status, right.Status)
	case "progress":
		return compareOrdered(left.Progress, right.Progress)
	case "startedAt":
		return left.StartedAt.Compare(right.StartedAt)
	default:
		return 0
	}
}

func compareOrdered[T ~int | ~float64](left, right T) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
