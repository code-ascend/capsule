// capsule
// Copyright (C) 2026 Дмитрий Удалов dmitry@udalov.online
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package clihelp

import (
	"context"
	"fmt"
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// Setup configures the global urfave/cli help machinery; call after i18n.Setup.
func Setup() {
	cli.HelpFlag = &cli.BoolFlag{
		Name: "help", Aliases: []string{"h"},
		Usage: gotext.Get("Show help"), HideDefault: true, Local: true,
	}
	cli.VersionFlag = &cli.BoolFlag{
		Name:  "version",
		Usage: gotext.Get("Print the version"), HideDefault: true, Local: true,
	}
	cli.UsageCommandHelp = gotext.Get("Shows a list of commands or help for one command")

	cli.RootCommandHelpTemplate = fmt.Sprintf(`%s
   {{template "helpNameTemplate" .}} {{if .Version}}{{if not .HideVersion}}{{.Version}}{{end}}{{end}}

%s
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} [command [command options]]{{end}}{{if .Description}}

%s
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

%s{{template "visibleCommandCategoryTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

%s{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s{{template "visibleFlagTemplate" .}}{{end}}
`,
		gotext.Get("Name:"),
		gotext.Get("Usage:"),
		gotext.Get("Description:"),
		gotext.Get("Commands:"),
		gotext.Get("Options:"),
		gotext.Get("Options:"),
	)

	cli.CommandHelpTemplate = fmt.Sprintf(`%s
   {{template "helpNameTemplate" .}}

%s
   {{template "usageTemplate" .}}{{if .Category}}

%s
   {{.Category}}{{end}}{{if .Description}}

%s
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

%s{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

%s{{template "visiblePersistentFlagTemplate" .}}{{end}}
`,
		gotext.Get("Name:"),
		gotext.Get("Usage:"),
		gotext.Get("Category:"),
		gotext.Get("Description:"),
		gotext.Get("Options:"),
		gotext.Get("Options:"),
		gotext.Get("Global options:"),
	)

	cli.SubcommandHelpTemplate = fmt.Sprintf(`%s
   {{template "helpNameTemplate" .}}

%s
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} [command [command options]]{{end}}{{if .Category}}

%s
   {{.Category}}{{end}}{{if .Description}}

%s
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

%s{{template "visibleCommandCategoryTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

%s{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

%s{{template "visiblePersistentFlagTemplate" .}}{{end}}
`,
		gotext.Get("Name:"),
		gotext.Get("Usage:"),
		gotext.Get("Category:"),
		gotext.Get("Description:"),
		gotext.Get("Commands:"),
		gotext.Get("Options:"),
		gotext.Get("Options:"),
		gotext.Get("Global options:"),
	)

	cli.FlagStringer = flagString
}

// SilenceUsageErrors makes usage errors flow to the exit path.
func SilenceUsageErrors(cmd *cli.Command) {
	cmd.OnUsageError = passUsageError
	for _, sub := range cmd.Commands {
		SilenceUsageErrors(sub)
	}
}

func passUsageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	return err
}

// flagString renders one flag help line: names, placeholder, usage, default and env hints.
func flagString(fl cli.Flag) string {
	df, ok := fl.(cli.DocGenerationFlag)
	if !ok {
		return ""
	}

	placeholder, usage := unquoteUsage(df.GetUsage())

	defaultHint := ""
	if rf, ok := fl.(cli.RequiredFlag); (!ok || !rf.IsRequired()) && df.IsDefaultVisible() {
		if s := df.GetDefaultText(); s != "" {
			defaultHint = fmt.Sprintf(" (%s: %s)", gotext.Get("default"), s)
		} else if df.TakesValue() && df.GetValue() != "" {
			defaultHint = fmt.Sprintf(" (%s: %s)", gotext.Get("default"), df.GetValue())
		}
	}

	var names strings.Builder
	for i, name := range fl.Names() {
		if name == "" {
			continue
		}
		if i > 0 {
			names.WriteString(", ")
		}
		if len(name) == 1 {
			names.WriteString("-" + name)
		} else {
			names.WriteString("--" + name)
		}
		if placeholder != "" {
			names.WriteString(" " + placeholder)
		}
	}
	prefixed := names.String()
	if mv, ok := fl.(cli.DocGenerationMultiValueFlag); ok && mv.IsMultiValueFlag() {
		prefixed += " [ " + prefixed + " ]"
	}

	envHint := ""
	if envVars := df.GetEnvVars(); len(envVars) > 0 {
		envHint = fmt.Sprintf(" [%s]", strings.Join(envVars, ", "))
	}

	return fmt.Sprintf("   %s\t%s%s", prefixed, strings.TrimSpace(usage+defaultHint), envHint)
}

// unquoteUsage extracts the `placeholder` from a flag usage string.
func unquoteUsage(usage string) (string, string) {
	if i := strings.IndexByte(usage, '`'); i >= 0 {
		if j := strings.IndexByte(usage[i+1:], '`'); j >= 0 {
			name := usage[i+1 : i+1+j]
			return name, usage[:i] + name + usage[i+2+j:]
		}
	}
	return "", usage
}
