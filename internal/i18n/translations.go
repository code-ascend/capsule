package i18n

import (
	"embed"
	"io/fs"
	"path"

	"github.com/jeandeaual/go-locale"
	"github.com/leonelquinteros/gotext"
)

//go:embed po
var poFS embed.FS

// Setup detects the user locale and registers it with gotext.
func Setup() {
	lang, err := locale.GetLanguage()
	if err != nil || lang == "" {
		return
	}
	if _, err := fs.Stat(poFS, path.Join("po", lang)); err != nil {
		return
	}
	loc := gotext.NewLocaleFSWithPath(lang, &poFS, "po")
	loc.SetDomain("default")
	gotext.SetLocales([]*gotext.Locale{loc})
}
