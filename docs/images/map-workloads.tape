# VHS tape for cub-scout map workloads screenshot
# Run from cub-scout repo root: vhs docs/images/map-workloads.tape

Output docs/images/map-workloads.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Run map and switch to workloads view
Type "cd /Users/alexis/Public/github-repos/cub-scout && ./cub-scout map"
Enter
Sleep 3s
Type "w"
Sleep 3s
Type "q"
