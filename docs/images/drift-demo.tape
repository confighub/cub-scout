# VHS tape for cub-scout drift/clobber screenshot
# Run from cub-scout repo root: vhs docs/images/drift-demo.tape

Output docs/images/drift-view.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Navigate and run, then switch to drift view
Type "cd /Users/alexis/Public/github-repos/cub-scout && ./cub-scout map"
Enter
Sleep 4s

# Press 'd' for drift view
Type "d"
Sleep 3s

Type "q"
