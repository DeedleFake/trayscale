package ui

import (
	"regexp"

	"deedles.dev/trayscale/internal/locale"
)

// Matches GtkBuilder/GMenu label attributes in embedded .ui XML.
var xmlLabelAttr = regexp.MustCompile(`(<attribute name="label">)([^<]*)(</attribute>)`)

// localizeXMLLabels rewrites label attributes through locale.Get so
// static menus (menu.ui) pick up catalog translations at load time.
func localizeXMLLabels(xml string) string {
	return xmlLabelAttr.ReplaceAllStringFunc(xml, func(match string) string {
		parts := xmlLabelAttr.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + locale.Get(parts[2]) + parts[3]
	})
}
