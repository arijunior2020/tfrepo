package cli

// bannerSubtitle is shown under the ASCII art when tfrepo is invoked
// without any arguments, mirroring SUBTITLE in apps/cli/src/banner.ts
// (TransfeRepo, commit 4bdbca7).
const bannerSubtitle = "Migração de repositórios Git entre provedores (GitHub, GitLab)"

// bannerArt is the ASCII art title shown when tfrepo is invoked without any
// arguments.
const bannerArt = `
████████╗███████╗██████╗ ███████╗██████╗   ██████╗
╚══██╔══╝██╔════╝██╔══██╗██╔════╝██╔══██╗ ██╔═══██╗
   ██║   █████╗  ██████╔╝█████╗  ██████╔╝ ██║   ██║
   ██║   ██╔══╝  ██╔══██╗██╔══╝  ██╔═══╝  ██║   ██║
   ██║   ██║     ██║  ██║███████╗██║      ╚██████╔╝
   ╚═╝   ╚═╝     ╚═╝  ╚═╝╚══════╝╚═╝       ╚═════╝
`

// renderBanner returns the ASCII art banner shown when tfrepo is invoked
// without any arguments.
func renderBanner() string {
	return bannerArt + bannerSubtitle + "\n"
}
