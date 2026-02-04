# panago
Golang code to explore and research panasonic bluray player

Build Instructions

Edit the .goreleaser.yaml for updating build targets

To test on local machine run

goreleaser release --snapshot --skip=publish --clean

This requires goreleaser, to test without use the build.sh script


To trigger an official release

#git tag v0.1.1

#git push origin v0.1.1

This will build and publish all versions of the binary in release section
 
 


## Enabling Interactive Shell (PTY Support)

The device has devpts kernel support but it's not mounted by default. This is why interactive shells don't work out of the box.

**To enable proper PTY/terminal support:**

```bash
mount -t devpts devpts /dev/pts
```

After this, you can:
- Use interactive shells via SSH (dropbear)
- Spawn proper terminal sessions
- Run programs that require a TTY

**Note:** The punch/shell workarounds in panago were created before discovering this. With devpts mounted, a simple dropbear SSH server provides full interactive access.

## Quick Setup Script

After gaining shell access, run:

```bash
# Enable PTY support
mkdir -p /dev/pts
mount -t devpts devpts /dev/pts

# Now interactive shells work
```
