# VHS demo tape for cub-scout
# Install: brew install vhs
# Run: vhs docs/images/demo.tape

Output docs/images/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Theme "Dracula"

# Show the command
Type "# Who owns every resource in my cluster?"
Sleep 500ms
Enter
Sleep 500ms

Type "./cub-scout map list | head -20"
Sleep 500ms
Enter
Sleep 3s

Type "# Trace a resource back to Git"
Sleep 500ms
Enter
Sleep 500ms

Type "./cub-scout trace deploy/cart -n boutique"
Sleep 500ms
Enter
Sleep 4s

Type "# Find orphaned resources"
Sleep 500ms
Enter
Sleep 500ms

Type "./cub-scout map orphans | head -10"
Sleep 500ms
Enter
Sleep 3s

Type "# Scan for misconfigurations"
Sleep 500ms
Enter
Sleep 500ms

Type "./cub-scout scan"
Sleep 500ms
Enter
Sleep 3s
