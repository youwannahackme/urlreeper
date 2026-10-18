package runner

import (
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/projectdiscovery/gologger"
	updateutils "github.com/projectdiscovery/utils/update"
)

var banner = (`
██╗   ██╗██████╗ ██╗     ██████╗ ███████╗███████╗██████╗ ███████╗██████╗ 
██║   ██║██╔══██╗██║     ██╔══██╗██╔════╝██╔════╝██╔══██╗██╔════╝██╔══██╗
██║   ██║██████╔╝██║     ██████╔╝█████╗  █████╗  ██████╔╝█████╗  ██████╔╝
██║   ██║██╔══██╗██║     ██╔══██╗██╔══╝  ██╔══╝  ██╔═══╝ ██╔══╝  ██╔══██╗
╚██████╔╝██║  ██║███████╗██║  ██║███████╗███████╗██║     ███████╗██║  ██║
 ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚══════╝╚══════╝╚═╝     ╚══════╝╚═╝  ╚═╝
`)

var version = "v1.7.0"

// ShowBanner displays the urlreeper banner to the user in purple.
func ShowBanner() {
	showBanner()
}

// showBanner is used to show the banner to the user
func showBanner() {
	noColor := os.Getenv("NO_COLOR") != ""
	if !noColor {
		for _, arg := range os.Args {
			if arg == "-nc" || arg == "-no-color" {
				noColor = true
				break
			}
		}
	}

	au := aurora.NewAurora(!noColor)
	purpleBanner := au.Index(135, banner).Bold().String()
	purpleAuthor := au.Index(141, "github.com/youwannahackme").String()
	purpleWhoami := au.Index(141, "Author: whoami_404").String()

	gologger.Print().Msgf("%s\n", purpleBanner)
	gologger.Print().Msgf("\t\t%s\n", purpleAuthor)
	gologger.Print().Msgf("\t\t%s\n\n", purpleWhoami)
}

// GetUpdateCallback returns a callback function that updates urlreeper
func GetUpdateCallback() func() {
	return func() {
		showBanner()
		updateutils.GetUpdateToolCallback("urlreeper", version)()
	}
}
