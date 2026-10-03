"""Fixed container probe; no model, tool dispatch, MCP, or network calls."""
import errno
import hashlib
import json
import os
from pathlib import Path
import stat


def main():
    root = Path('/tools')
    files = {}
    total = 0
    for current, directories, names in os.walk(root, followlinks=False):
        for name in directories + names:
            path = Path(current) / name
            mode = path.lstat().st_mode
            if not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)):
                raise ValueError('nonregular stage entry')
        for name in names:
            path = Path(current) / name
            if path.stat().st_size > 32 * 1024 * 1024:
                raise ValueError('file exceeds probe bound')
            data = path.read_bytes()
            total += len(data)
            if total > 64 * 1024 * 1024 or len(files) >= 4096:
                raise ValueError('stage exceeds probe bound')
            files[path.relative_to(root).as_posix()] = hashlib.sha256(data).hexdigest()
    if not files or os.getuid() != 65534 or os.getgid() != 65534:
        raise ValueError('empty stage or unexpected user')
    denied = []
    os.symlink('/tools', '/tmp/stage-alias')
    for target in ('/tools/.write-control', '/tmp/stage-alias/.write-control', '/root-write-control'):
        try:
            fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except OSError as exc:
            if exc.errno not in (errno.EROFS, errno.EACCES):
                raise
            denied.append(target)
        else:
            os.close(fd)
            raise ValueError('write unexpectedly succeeded')
    interfaces = [line.split(':', 1)[0].strip() for line in Path('/proc/net/dev').read_text().splitlines()[2:]]
    if interfaces != ['lo']:
        raise ValueError('unexpected network interface')
    print(json.dumps({'schema': 'full24-case-isolation-probe.v1', 'files': files,
                      'bytes': total, 'uid': os.getuid(), 'gid': os.getgid(),
                      'writeDenied': denied, 'interfaces': interfaces}, sort_keys=True))


if __name__ == '__main__':
    main()
