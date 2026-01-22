# VHS tape for cub-scout TUI screenshot
# Run: vhs docs/images/screenshot.tape

Output docs/images/map-dashboard.png

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Launch TUI
Type "cub-scout map"
Enter
Sleep 3s

# Take screenshot of initial view
Screenshot docs/images/map-dashboard.png
