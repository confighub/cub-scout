# VHS tape for cub-scout TUI screenshot
# Run from cub-scout repo root: vhs docs/images/screenshot.tape
#
# Note: VHS creates a directory of frames. To get a single PNG:
#   1. Run this tape
#   2. Pick a frame: cp docs/images/map-dashboard.png/frame-text-00406.png /tmp/shot.png
#   3. Replace: rm -rf docs/images/map-dashboard.png && mv /tmp/shot.png docs/images/map-dashboard.png

Output docs/images/map-dashboard.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Navigate and run
Type "cd /Users/alexis/Public/github-repos/cub-scout && ./cub-scout map"
Enter
Sleep 5s

Type "q"
