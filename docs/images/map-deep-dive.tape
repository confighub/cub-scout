# VHS tape for cub-scout map deep-dive screenshot
# Run from cub-scout repo root: vhs docs/images/map-deep-dive.tape

Output docs/images/map-deep-dive.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Run map and switch to deep-dive view (tab 4)
Type "cd /Users/alexis/Public/github-repos/cub-scout && ./cub-scout map"
Enter
Sleep 3s
Type "4"
Sleep 3s
Type "q"
