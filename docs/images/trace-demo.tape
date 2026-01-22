# VHS tape for cub-scout trace ownership screenshot
# Run from cub-scout repo root: vhs docs/images/trace-demo.tape

Output docs/images/trace-ownership.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Run trace command on a Flux-managed resource
Type "cd /Users/alexis/Public/github-repos/cub-scout && ./cub-scout trace deploy/order-service-traditional -n demo-traditional"
Enter
Sleep 4s
