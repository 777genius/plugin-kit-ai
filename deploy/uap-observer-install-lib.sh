#!/bin/sh

# Shared, testable installer primitives. The caller supplies only disposable
# roots in tests; production calls use the fixed system paths.

observer_units='uap-observer.service uap-observer-signer.service uap-observer-runner.service uap-observer-runner.socket uap-observer-caddy.service uap-observer-egress-proxy.service uap-observer-egress-proxy.socket'

# Control data is never consumed through a pathname-opening utility.  The
# parent and object are pinned without following links and O_NOATIME keeps
# validation and recovery from becoming metadata mutations themselves.
observer_read_control_file() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,stat,sys
path=os.path.abspath(sys.argv[1]); parent_path=os.path.dirname(path); name=os.path.basename(path)
dirflags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
fileflags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
parent_info=os.stat(parent_path,follow_symlinks=False); parent=os.open(parent_path,dirflags)
try:
    if os.fstat(parent)!=parent_info: raise PermissionError("control parent changed while opening")
    before=os.stat(name,dir_fd=parent,follow_symlinks=False); descriptor=os.open(name,fileflags,dir_fd=parent)
    try:
        if os.fstat(descriptor)!=before or not stat.S_ISREG(before.st_mode): raise PermissionError("control file changed while opening")
        while True:
            block=os.read(descriptor,1<<20)
            if not block: break
            os.write(1,block)
        if os.fstat(descriptor)!=before: raise PermissionError("control file changed while reading")
    finally: os.close(descriptor)
finally: os.close(parent)
PY
}

# Compare regular files without changing either file's access time.  This is
# used on the already-installed fast path, where validation must be a literal
# metadata no-op rather than relying on cmp(1)'s ordinary read opens.
observer_compare_regular_files_neutral() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" "$2" <<'PY'
import os,stat,sys
flags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
descriptors=[]
try:
    before=[]
    for path in sys.argv[1:]:
        info=os.stat(path,follow_symlinks=False)
        if not stat.S_ISREG(info.st_mode): raise PermissionError("comparison target is not a regular file")
        descriptor=os.open(path,flags); descriptors.append(descriptor)
        if os.fstat(descriptor)!=info: raise PermissionError("comparison target changed while opening")
        before.append(info)
    while True:
        left=os.read(descriptors[0],1<<20); right=os.read(descriptors[1],1<<20)
        if left!=right: raise SystemExit(1)
        if not left: break
    if any(os.fstat(descriptor)!=info for descriptor,info in zip(descriptors,before)):
        raise PermissionError("comparison target changed while reading")
    if any(os.stat(path,follow_symlinks=False)!=info for path,info in zip(sys.argv[1:],before)):
        raise PermissionError("comparison target pathname changed while reading")
finally:
    for descriptor in reversed(descriptors): os.close(descriptor)
PY
}

observer_sha256_regular_neutral() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import hashlib,os,stat,sys
path=sys.argv[1]; flags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
before=os.stat(path,follow_symlinks=False)
if not stat.S_ISREG(before.st_mode): raise PermissionError("digest target is not a regular file")
descriptor=os.open(path,flags)
try:
    if os.fstat(descriptor)!=before: raise PermissionError("digest target changed while opening")
    digest=hashlib.sha256()
    while True:
        block=os.read(descriptor,1<<20)
        if not block: break
        digest.update(block)
    if os.fstat(descriptor)!=before or os.stat(path,follow_symlinks=False)!=before:
        raise PermissionError("digest target changed while reading")
    print(digest.hexdigest())
finally: os.close(descriptor)
PY
}

# Execute a validator with the authenticated closure exposed through a private
# read-only, no-atime bind mount.  Python's import machinery cannot request
# O_NOATIME itself, so -B alone is insufficient for the identical-install
# metadata no-op contract.
observer_run_closure_python_neutral() {
  closure=$1
  shift
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$closure" "$@" <<'PY'
import ctypes,os,sys
closure=os.fsencode(os.path.abspath(sys.argv[1])); command=sys.argv[2:]
libc=ctypes.CDLL(None,use_errno=True)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18
MS_NOATIME=1024; MS_NODIRATIME=2048
def call(result,label):
    if result:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
call(libc.mount(closure,closure,None,MS_BIND,None),"bind authenticated closure")
call(libc.mount(None,closure,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOSUID|MS_NODEV|MS_NOATIME|MS_NODIRATIME,None),"remount authenticated closure no-atime")
environment=os.environ.copy(); environment["PYTHONDONTWRITEBYTECODE"]="1"
os.execvpe(command[0],command,environment)
PY
}

observer_run_closure_python_script_neutral() {
  closure=$1
  interpreter=$2
  shift 2
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$closure" "$interpreter" "$@" 8<&0 <<'PY'
import ctypes,os,sys
closure=os.fsencode(os.path.abspath(sys.argv[1])); code=os.fdopen(8).read()
arguments=sys.argv[3:]
if arguments[:1]==["-B"]: arguments=arguments[1:]
command=[sys.argv[2],"-B","-c",code,*arguments]
libc=ctypes.CDLL(None,use_errno=True)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18
MS_NOATIME=1024; MS_NODIRATIME=2048
def call(result,label):
    if result:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
call(libc.mount(closure,closure,None,MS_BIND,None),"bind authenticated closure")
call(libc.mount(None,closure,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOSUID|MS_NODEV|MS_NOATIME|MS_NODIRATIME,None),"remount authenticated closure no-atime")
environment=os.environ.copy(); environment["PYTHONDONTWRITEBYTECODE"]="1"
os.execvpe(command[0],command,environment)
PY
}

observer_directory_inventory_neutral() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" "${2:-}" <<'PY'
import os,stat,sys
path=os.path.abspath(sys.argv[1]); prefix=sys.argv[2]
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
before=os.stat(path,follow_symlinks=False)
if not stat.S_ISDIR(before.st_mode): raise PermissionError("inventory root is unsafe")
root=os.open(path,flags)
try:
    if os.fstat(root)!=before: raise PermissionError("inventory root changed while opening")
    for name in sorted(name for name in os.listdir(root) if name.startswith(prefix)):
        os.write(1,os.fsencode(name)+b"\n")
    if os.fstat(root)!=before: raise PermissionError("inventory root changed while reading")
finally:
    os.close(root)
PY
}

observer_read_symlink_neutral() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import ctypes,os,signal,stat,sys,tempfile
path=os.path.abspath(sys.argv[1]); parent_path=os.path.dirname(path); name=os.path.basename(path)
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
libc=ctypes.CDLL(None,use_errno=True)
libc.mount.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_char_p,ctypes.c_ulong,ctypes.c_void_p)
libc.umount2.argtypes=(ctypes.c_char_p,ctypes.c_int)
libc.readlinkat.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4; MS_NOEXEC=8
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18
MS_NOATIME=1024; MS_NODIRATIME=2048; MNT_DETACH=2
def call(result,label):
    if result != 0:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
if os.environ.get("UAP_OBSERVER_TEST_NOATIME_UNSUPPORTED")=="1": raise OSError("private no-atime view unavailable")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
parent_info=os.stat(parent_path,follow_symlinks=False); parent=os.open(parent_path,flags)
def metadata(value):
    return (value.st_dev,value.st_ino,stat.S_IFMT(value.st_mode),stat.S_IMODE(value.st_mode),
            value.st_uid,value.st_gid,value.st_nlink,value.st_atime_ns,value.st_mtime_ns,value.st_ctime_ns)
def neutral_readlink(parentfd,name,before):
    # O_PATH ignores O_NOATIME.  Pin the parent first, then expose that exact
    # directory only through a private, read-only no-atime bind mount.
    view=tempfile.mkdtemp(prefix="uap-observer-noatime-")
    encoded=os.fsencode(view); mounted=False
    try:
        source=b"/proc/self/fd/"+str(parentfd).encode("ascii")
        call(libc.mount(source,encoded,None,MS_BIND,None),"bind pinned symlink parent")
        mounted=True
        call(libc.mount(None,encoded,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOATIME|MS_NODIRATIME,None),"remount pinned symlink parent noatime")
        viewfd=os.open(view,flags)
        try:
            linkfd=os.open(name,os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=viewfd)
            try:
                if metadata(os.fstat(linkfd))!=metadata(before): raise PermissionError("control symlink changed while pinning")
                ready_fd=os.environ.get("UAP_OBSERVER_TEST_SYMLINK_PIN_READY_FD")
                if ready_fd:
                    os.write(int(ready_fd),b"1")
                    os.kill(os.getpid(),signal.SIGSTOP)
                size=max(256,before.st_size+1)
                while True:
                    buffer=ctypes.create_string_buffer(size)
                    length=libc.readlinkat(linkfd,b"",buffer,size)
                    if length < 0:
                        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
                    if length < size: break
                    size*=2
                if metadata(os.fstat(linkfd))!=metadata(before): raise PermissionError("control symlink changed while reading")
                return os.fsdecode(buffer.raw[:length])
            finally: os.close(linkfd)
        finally: os.close(viewfd)
    finally:
        if mounted: call(libc.umount2(encoded,MNT_DETACH),"unmount noatime view")
        os.rmdir(view)
try:
    if os.fstat(parent)!=parent_info: raise PermissionError("symlink parent changed while opening")
    before=os.stat(name,dir_fd=parent,follow_symlinks=False)
    if not stat.S_ISLNK(before.st_mode): raise PermissionError("control symlink is unsafe")
    value=neutral_readlink(parent,name,before)
    after=os.stat(name,dir_fd=parent,follow_symlinks=False)
    if metadata(after)!=metadata(before): raise PermissionError("control symlink changed while reading")
    os.write(1,os.fsencode(value))
finally: os.close(parent)
PY
}

# This is the authoritative inventory of every temporary name created by the
# installer.  Both retry cleanup and the identical-install trust check consume
# it so a newly added staging path cannot silently escape one of them.
observer_partial_paths() {
  printf '%s\n' \
    /opt/uap-observer-source.new \
    /opt/uap-observer-venv.new \
    /opt/uap-observer-runtime.new \
    /opt/uap-observer-current.new \
    /usr/local/libexec/uap-observer-runner.new \
    /usr/local/libexec/uap-observer-egress-proxy.new \
    /usr/local/libexec/uap-observer-fixed-adapter.new \
    /usr/local/libexec/uap-observer-attest-chatgpt.new \
    /usr/local/libexec/uap-observer-attest-consent.new \
    /usr/local/libexec/uap-observer-provision-profile.new \
    /usr/local/bin/caddy.new \
    /etc/uap-observer.json.new \
    /etc/uap-observer-egress-allowlist.json.new \
    /etc/uap-observer-adapter-config.json.new \
    /etc/uap-observer-adapters.json.new \
    /etc/caddy/Caddyfile.new \
    /opt/uap-observer-closures/.new-* \
    /usr/local/libexec/uap-observer-adapter-*.new
}

observer_validate_no_partial_paths() {
  inventory=${1:-observer_partial_paths}
  for partial in $("$inventory"); do
    test ! -e "$partial"
    test ! -L "$partial"
  done
}

observer_cleanup_partial_paths() {
  inventory=${1:-observer_partial_paths}
  for partial in $("$inventory"); do
    if [ -e "$partial" ] || [ -L "$partial" ]; then rm -rf -- "$partial"; fi
  done
}

# Recovery owns the journal directory and removes it only after every other
# cleanup and durability operation has succeeded.
observer_cleanup_recovery_partials() {
  inventory=${1:-observer_partial_paths}
  cleaned_parents=
  for partial in $("$inventory"); do
    test "$partial" = /opt/uap-observer-source.new && continue
    parent=$(dirname "$partial")
    case " $cleaned_parents " in *" $parent "*) ;; *) cleaned_parents="$cleaned_parents $parent";; esac
    if [ -e "$partial" ] || [ -L "$partial" ]; then
      rm -rf -- "$partial" || return 1
    fi
  done
  for parent in $cleaned_parents; do observer_sync_directory "$parent" || return 1; done
}

observer_validate_first_install_closures_root() {
  closures_root=$1
  expected_owner=${2:-0:0}
  if [ ! -e "$closures_root" ] && [ ! -L "$closures_root" ]; then return 0; fi
  test -d "$closures_root" || return 1
  test ! -L "$closures_root" || return 1
  test "$(stat -c '%u:%g:%a' "$closures_root")" = "$expected_owner:755" || return 1
  test -z "$(find "$closures_root" -mindepth 1 -maxdepth 1 -print -quit)"
}

# venv creates its own pip/setuptools cache before pip's --no-compile policy
# can take effect.  Remove only bytecode/cache names through pinned directory
# descriptors, with hard traversal bounds and without ever following a link.
observer_remove_python_bytecode() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,stat,sys
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
root_before=os.stat(sys.argv[1],follow_symlinks=False); root=os.open(sys.argv[1],flags)
count=0
def consume():
    global count
    count+=1
    if count>200000: raise RuntimeError("bytecode cleanup entry bound exceeded")
def remove_tree(parent,name,depth):
    if depth>128: raise RuntimeError("bytecode cleanup depth bound exceeded")
    before=os.stat(name,dir_fd=parent,follow_symlinks=False)
    if stat.S_ISDIR(before.st_mode):
        child=os.open(name,flags,dir_fd=parent)
        try:
            if os.fstat(child)!=before: raise PermissionError("bytecode cache raced while opening")
            for entry in os.listdir(child): consume(); remove_tree(child,entry,depth+1)
            stable=lambda x:(x.st_dev,x.st_ino,stat.S_IFMT(x.st_mode),stat.S_IMODE(x.st_mode),x.st_uid,x.st_gid)
            if stable(os.fstat(child))!=stable(before) or stable(os.stat(name,dir_fd=parent,follow_symlinks=False))!=stable(before):
                raise PermissionError("bytecode cache raced while deleting")
        finally: os.close(child)
        os.rmdir(name,dir_fd=parent)
    else:
        os.unlink(name,dir_fd=parent)
def walk(directory,depth=0):
    if depth>128: raise RuntimeError("venv traversal depth bound exceeded")
    before=os.fstat(directory)
    for name in os.listdir(directory):
        consume(); info=os.stat(name,dir_fd=directory,follow_symlinks=False)
        if name=="__pycache__": remove_tree(directory,name,depth+1)
        elif stat.S_ISREG(info.st_mode) and name.endswith((".pyc",".pyo")):
            os.unlink(name,dir_fd=directory)
        elif stat.S_ISDIR(info.st_mode):
            child=os.open(name,flags,dir_fd=directory)
            try:
                if os.fstat(child)!=info: raise PermissionError("venv directory raced while opening")
                walk(child,depth+1)
            finally: os.close(child)
    # Directory mtime changes are expected only when this call removed one of
    # its direct cache children; identity/type/owner/mode still may not race.
    after=os.fstat(directory)
    stable=lambda x:(x.st_dev,x.st_ino,stat.S_IFMT(x.st_mode),stat.S_IMODE(x.st_mode),x.st_uid,x.st_gid)
    if stable(after)!=stable(before): raise PermissionError("venv directory raced during cleanup")
try:
    if os.fstat(root)!=root_before: raise PermissionError("venv root raced while opening")
    walk(root)
finally: os.close(root)
PY
}

# Published closure mtimes are stable authority, not build-clock noise.  Use
# one nanosecond-exact value and descriptor-relative operations for files,
# directories and links.  This helper is also applied to the production
# systemd stage before it is copied to the closure or activated.
observer_normalize_tree_mtime() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,stat,sys
STAMP=1700000000123456789
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
root=os.open(sys.argv[1],flags); count=0
def walk(directory,depth=0):
    global count
    if depth>128: raise RuntimeError("mtime normalization depth bound exceeded")
    for name in os.listdir(directory):
        count+=1
        if count>200000: raise RuntimeError("mtime normalization entry bound exceeded")
        info=os.stat(name,dir_fd=directory,follow_symlinks=False)
        if stat.S_ISDIR(info.st_mode):
            child=os.open(name,flags,dir_fd=directory)
            try:
                if os.fstat(child)!=info: raise PermissionError("tree raced during mtime normalization")
                walk(child,depth+1); os.utime(child,ns=(STAMP,STAMP))
            finally: os.close(child)
        elif stat.S_ISREG(info.st_mode):
            descriptor=os.open(name,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=directory)
            try:
                if os.fstat(descriptor)!=info: raise PermissionError("file raced during mtime normalization")
                os.utime(descriptor,ns=(STAMP,STAMP))
            finally: os.close(descriptor)
        elif stat.S_ISLNK(info.st_mode):
            os.utime(name,ns=(STAMP,STAMP),dir_fd=directory,follow_symlinks=False)
        else: raise PermissionError("tree contains unsafe object during mtime normalization")
try:
    walk(root); os.utime(root,ns=(STAMP,STAMP))
finally: os.close(root)
PY
}

observer_closure_identity() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import ctypes,hashlib,os,stat,sys,tempfile
root_path=os.path.abspath(sys.argv[1])
dirflags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
fileflags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
libc=ctypes.CDLL(None,use_errno=True)
libc.mount.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_char_p,ctypes.c_ulong,ctypes.c_void_p)
libc.umount2.argtypes=(ctypes.c_char_p,ctypes.c_int)
libc.readlinkat.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4; MS_NOEXEC=8
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18
MS_NOATIME=1024; MS_NODIRATIME=2048; MNT_DETACH=2
namespace_ready=False
def call(result,label):
    if result != 0:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
namespace_ready=True
root_before=os.stat(root_path,follow_symlinks=False); root=os.open(root_path,dirflags)
if os.fstat(root)!=root_before: raise PermissionError("closure root changed while opening")
def metadata(value):
    return (value.st_dev,value.st_ino,stat.S_IFMT(value.st_mode),stat.S_IMODE(value.st_mode),
            value.st_uid,value.st_gid,value.st_nlink,value.st_atime_ns,value.st_mtime_ns,value.st_ctime_ns)
def attributes(parent,name,info,descriptor=None):
    if stat.S_ISLNK(info.st_mode):
        target=f"/proc/self/fd/{parent}/{name}"; follow=False
    else:
        target=descriptor; follow=True
    names=sorted(os.listxattr(target,follow_symlinks=follow),key=os.fsencode)
    result=[]
    for key in names:
        result.append((os.fsencode(key),os.getxattr(target,key,follow_symlinks=follow)))
    return tuple(result)
root_attrs=attributes(None,None,root_before,root)
def neutral_readlink(parent,name,before):
    global namespace_ready
    if not namespace_ready:
        call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
        call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
        namespace_ready=True
    view=tempfile.mkdtemp(prefix="uap-observer-noatime-"); encoded=os.fsencode(view); mounted=False
    try:
        call(libc.mount(b"/proc/self/fd/"+str(parent).encode(),encoded,None,MS_BIND,None),"bind pinned closure directory"); mounted=True
        call(libc.mount(None,encoded,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOATIME|MS_NODIRATIME,None),"remount closure directory noatime")
        viewfd=os.open(view,dirflags)
        try:
            linkfd=os.open(name,os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=viewfd)
            try:
                if metadata(os.fstat(linkfd))!=metadata(before): raise PermissionError("closure symlink changed while pinning")
                size=max(256,before.st_size+1)
                while True:
                    buffer=ctypes.create_string_buffer(size); length=libc.readlinkat(linkfd,b"",buffer,size)
                    if length < 0:
                        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
                    if length < size: break
                    size*=2
                if metadata(os.fstat(linkfd))!=metadata(before): raise PermissionError("closure symlink changed while reading")
                return buffer.raw[:length]
            finally: os.close(linkfd)
        finally: os.close(viewfd)
    finally:
        if mounted: call(libc.umount2(encoded,MNT_DETACH),"unmount closure noatime view")
        os.rmdir(view)
entries={}
def walk(directory,prefix=""):
    before=os.fstat(directory)
    for name in sorted(os.listdir(directory)):
        if name == "__pycache__" or name.endswith((".pyc",".pyo")):
            raise SystemExit("closure contains Python bytecode/cache")
        relative=name if not prefix else prefix+"/"+name
        info=os.stat(name,dir_fd=directory,follow_symlinks=False)
        if stat.S_ISDIR(info.st_mode):
            child=os.open(name,dirflags,dir_fd=directory)
            try:
                if os.fstat(child)!=info: raise PermissionError("closure directory changed while opening")
                attrs=attributes(directory,name,info,child)
                if os.fstat(child)!=info or os.stat(name,dir_fd=directory,follow_symlinks=False)!=info: raise PermissionError("closure directory changed while reading attributes")
                entries[relative]=(info,b"d",None,attrs)
                walk(child,relative)
                if os.fstat(child)!=info or os.stat(name,dir_fd=directory,follow_symlinks=False)!=info or attributes(directory,name,info,child)!=attrs: raise PermissionError("closure directory changed while traversing")
            finally: os.close(child)
        elif stat.S_ISREG(info.st_mode):
            descriptor=os.open(name,fileflags,dir_fd=directory)
            try:
                if os.fstat(descriptor)!=info: raise PermissionError("closure file changed while opening")
                digest=hashlib.sha256()
                while True:
                    block=os.read(descriptor,1<<20)
                    if not block: break
                    digest.update(block)
                if os.fstat(descriptor)!=info: raise PermissionError("closure file changed while reading")
                attrs=attributes(directory,name,info,descriptor)
                if os.fstat(descriptor)!=info or os.stat(name,dir_fd=directory,follow_symlinks=False)!=info or attributes(directory,name,info,descriptor)!=attrs: raise PermissionError("closure file changed while reading attributes")
                entries[relative]=(info,b"f",digest.digest(),attrs)
            finally: os.close(descriptor)
        elif stat.S_ISLNK(info.st_mode):
            target=neutral_readlink(directory,name,info)
            if metadata(os.stat(name,dir_fd=directory,follow_symlinks=False))!=metadata(info): raise PermissionError("closure symlink changed while reading")
            attrs=attributes(directory,name,info)
            if metadata(os.stat(name,dir_fd=directory,follow_symlinks=False))!=metadata(info) or attributes(directory,name,info)!=attrs: raise PermissionError("closure symlink changed while reading attributes")
            entries[relative]=(info,b"l",target,attrs)
        else: entries[relative]=(info,b"?",None,())
    if os.fstat(directory)!=before: raise PermissionError("closure directory changed during traversal")
walk(root)
identity=hashlib.sha256()
regular={}
for relative,(info,kind,payload,attrs) in entries.items():
    if kind==b"f": regular.setdefault((info.st_dev,info.st_ino),[]).append(relative)
adapter_paths={f"libexec/uap-observer-adapter-{name}" for name in ("runtime","notion","chatgpt","consent")}
adapter_paths.add("libexec/uap-observer-fixed-adapter")
for inode,members in regular.items():
    members.sort()
    info=entries[members[0]][0]
    actual=set(members)
    if actual == adapter_paths:
        if info.st_nlink != 5: raise SystemExit("adapter hardlink set has an external link")
    elif len(members) != 1 or info.st_nlink != 1:
        raise SystemExit("closure regular file has unexpected hardlink topology")
groups=[set(members) for members in regular.values() if adapter_paths & set(members)]
if groups != [adapter_paths]: raise SystemExit("adapter paths are not one exact hardlink set")
def framed(value): return len(value).to_bytes(8,"big")+value
identity.update(b"root\0"+str(stat.S_IMODE(root_before.st_mode)).encode()+b"\0"+str(root_before.st_uid).encode()+b"\0"+str(root_before.st_gid).encode()+b"\0mtime-ns\0"+str(root_before.st_mtime_ns).encode()+b"\0xattrs"+len(root_attrs).to_bytes(8,"big"))
for key,value in root_attrs: identity.update(framed(key)+framed(value))
for name in sorted(entries):
    info,kind,payload,attrs=entries[name]; relative=name.encode()
    identity.update(b"\0".join((relative,kind,str(stat.S_IMODE(info.st_mode)).encode(),str(info.st_uid).encode(),str(info.st_gid).encode(),b"mtime-ns",str(info.st_mtime_ns).encode()))+b"\0")
    identity.update(b"xattrs"+len(attrs).to_bytes(8,"big"))
    for key,value in attrs: identity.update(framed(key)+framed(value))
    if kind == b"f":
        # Canonical path equivalence classes bind topology without hashing raw,
        # filesystem-specific inode numbers.
        topology="=".join(regular[(info.st_dev,info.st_ino)]).encode()
        identity.update(b"links\0"+topology+b"\0"+payload)
    elif kind == b"l": identity.update(payload)
    elif kind == b"?": raise SystemExit("closure contains a special file")
if os.fstat(root)!=root_before or attributes(None,None,root_before,root)!=root_attrs: raise PermissionError("closure root changed during identity traversal")
os.close(root)
print(identity.hexdigest())
PY
}

observer_install_input_identity() {
  source_root=$1
  runtime_manifest_digest=$2
  adapter_config=$3
  adapter_config_digest=$4
  observer_config=$5
  observer_config_digest=$6
  caddy_archive=$7
  caddy_archive_digest=$8
  caddy_config=$9
  shift 9
  caddy_config_digest=$1
  egress_allowlist=$2
  egress_allowlist_digest=$3
  manifest="$source_root/deploy/uap-observer-runtime.sha256"
  test -d "$source_root"
  test ! -L "$source_root"
  test -f "$manifest"
  test ! -L "$manifest"
  test "$(sha256sum "$manifest" | cut -d' ' -f1)" = "$runtime_manifest_digest"
  while read -r expected relative extra; do
    test -z "${extra:-}"
    case "$expected:$relative" in
      [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*:[A-Za-z0-9._-]*) ;;
      *) echo "invalid runtime closure entry" >&2; return 1 ;;
    esac
    case "/$relative/" in */../*|*/./*|//* ) echo "unsafe runtime closure path" >&2; return 1 ;; esac
    test -f "$source_root/$relative"
    test ! -L "$source_root/$relative"
  done < "$manifest"
  (cd "$source_root" && sha256sum -c deploy/uap-observer-runtime.sha256 >/dev/null)
  for input in "$adapter_config" "$observer_config" "$caddy_archive" "$caddy_config" "$egress_allowlist"; do
    test -f "$input"
    test ! -L "$input"
  done
  actual_adapter="sha256:$(sha256sum "$adapter_config" | cut -d' ' -f1)"
  actual_observer="sha256:$(sha256sum "$observer_config" | cut -d' ' -f1)"
  actual_archive="sha256:$(sha256sum "$caddy_archive" | cut -d' ' -f1)"
  actual_caddy="sha256:$(sha256sum "$caddy_config" | cut -d' ' -f1)"
  actual_egress="sha256:$(sha256sum "$egress_allowlist" | cut -d' ' -f1)"
  test "$actual_adapter" = "$adapter_config_digest"
  test "$actual_observer" = "$observer_config_digest"
  test "$actual_archive" = "sha256:$caddy_archive_digest"
  test "$actual_caddy" = "$caddy_config_digest"
  test "$actual_egress" = "$egress_allowlist_digest"
  printf '%s\n' \
    "runtime-manifest sha256:$runtime_manifest_digest" \
    "adapter-config $actual_adapter" \
    "observer-config $actual_observer" \
    "caddy-archive $actual_archive" \
    "caddy-config $actual_caddy" \
    "egress-allowlist $actual_egress" | sha256sum | cut -d' ' -f1
}

observer_validate_completed_closure() {
  closures_root=$1
  current_pointer=$2
  expected_install_identity=$3
  expected_owner=${4:-0:0}
  systemd_root=${5:-/etc/systemd/system}
  config_gid=${6:-$(getent group uap-observer-adapter-config | cut -d: -f3)}
  caddy_gid=${7:-$(getent group caddy | cut -d: -f3)}
  test -n "$config_gid"
  test -n "$caddy_gid"
  test -d "$closures_root"
  test ! -L "$closures_root"
  test "$(stat -c '%u:%g:%a' "$closures_root")" = "$expected_owner:755"
  test -L "$current_pointer"
  test "$(stat -c '%u:%g:%a' "$current_pointer")" = "$expected_owner:777"
  target=$(observer_read_symlink_neutral "$current_pointer")
  digest=${target#uap-observer-closures/}
  printf '%s\n' "$digest" | grep -Eq '^[0-9a-f]{64}$' || {
    echo "observer current pointer is invalid" >&2
    return 1
  }
  test "$target" = "uap-observer-closures/$digest"
  closure="$closures_root/$digest"
  test "$(observer_directory_inventory_neutral "$closures_root")" = "$digest"
  test -d "$closure"
  test ! -L "$closure"
  test "$(stat -c '%u:%g:%a' "$closure")" = "$expected_owner:755"
  for marker in .complete .install-identity; do
    test -f "$closure/$marker"
    test ! -L "$closure/$marker"
    test "$(stat -c '%u:%g:%a' "$closure/$marker")" = "$expected_owner:644"
  done
  test "$(observer_read_control_file "$closure/.complete")" = complete-v1
  test "$(observer_read_control_file "$closure/.install-identity")" = "$expected_install_identity"
  actual_identity=$(observer_closure_identity "$closure")
  test "$actual_identity" = "$digest"
  test "$(stat -c '%u:%g:%a:%h' "$closure/etc/uap-observer-adapter-config.json")" = "0:$config_gid:640:1"
  test "$(stat -c '%u:%g:%a:%h' "$closure/etc/Caddyfile")" = "0:$caddy_gid:640:1"
  test "$(stat -c '%u:%g:%a:%h' "$closure/etc/uap-observer-egress-allowlist.json")" = "0:0:644:1"
  test "$(stat -c '%u:%g:%a:%h' "$closure/libexec/uap-observer-egress-proxy")" = "0:0:755:1"
  test -d "$systemd_root"
  test ! -L "$systemd_root"
  test "$(stat -c '%u:%g:%a' "$systemd_root")" = "$expected_owner:755"
  for unit in $observer_units; do
    installed="$systemd_root/$unit"
    reviewed="$closure/systemd/$unit"
    test -f "$installed"
    test ! -L "$installed"
    test "$(stat -c '%h' "$installed")" = 1
    test "$(stat -c '%u:%g:%a' "$installed")" = "$expected_owner:644"
    observer_compare_regular_files_neutral "$reviewed" "$installed"
  done
  for service in uap-observer uap-observer-runner; do
    installed="$systemd_root/$service.service.d"
    reviewed="$closure/systemd/$service.service.d/egress.conf"
    test -d "$installed"
    test ! -L "$installed"
    test "$(stat -c '%u:%g:%a' "$installed")" = "$expected_owner:755"
    test "$(observer_directory_inventory_neutral "$installed")" = egress.conf
    test -f "$installed/egress.conf"
    test ! -L "$installed/egress.conf"
    test "$(stat -c '%h' "$installed/egress.conf")" = 1
    test "$(stat -c '%u:%g:%a' "$installed/egress.conf")" = "$expected_owner:644"
    observer_compare_regular_files_neutral "$reviewed" "$installed/egress.conf"
  done
  expected_observer_paths=$(printf '%s\n' $observer_units uap-observer.service.d uap-observer-runner.service.d | LC_ALL=C sort)
  actual_observer_paths=$(observer_directory_inventory_neutral "$systemd_root" uap-observer)
  test "$actual_observer_paths" = "$expected_observer_paths"
  observer_compare_systemd_trees "$closure/systemd" "$systemd_root"
}

observer_validate_installed_accounts_and_state() {
  closure=${1:-/opt/uap-observer-current}
  observer_runtime="$closure/runtime"
  PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$observer_runtime" observer_run_closure_python_script_neutral "$closure" python3 -B <<'PY'
import grp,hashlib,json,math,os,pwd,re,stat
from pathlib import Path
from observer.fixed_runner import reviewed_service_identities

services=reviewed_service_identities()
identities=[services[name][:2] for name in ("codex","cursor","kiro","control")]
observer_uid,observer_gid,_=services["observer"]
caddy_uid,caddy_gid,_=services["caddy"]
egress_uid,egress_gid,_=services["egress"]
if egress_uid == 0 or egress_gid == 0: raise SystemExit("egress gateway is privileged")

def directory(path,uid,gid,mode):
    info=os.lstat(path)
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != uid or info.st_gid != gid or stat.S_IMODE(info.st_mode) != mode:
        raise SystemExit(f"installed state directory {path} differs")

def native_projection(encoded,suffix,profile,proof):
    if len(encoded) > 4 << 20:
        raise SystemExit(f"installed native projection for {suffix} is oversized")
    def pairs(items):
        result={}; folded=set()
        for key,value in items:
            normalized=key.casefold()
            if key in result or normalized in folded:
                raise SystemExit(f"installed native projection for {suffix} has duplicate or case-confusable members")
            result[key]=value; folded.add(normalized)
        return result
    def constant(_value):
        raise SystemExit(f"installed native projection for {suffix} has a non-finite number")
    def finite(value):
        decoded=float(value)
        if not math.isfinite(decoded):
            raise SystemExit(f"installed native projection for {suffix} has a non-finite number")
        return decoded
    value=json.loads(encoded,object_pairs_hook=pairs,parse_constant=constant,parse_float=finite)
    evidence={"manager_add_sha256","manager_info_sha256","post_add_doctor_sha256"}
    fields={"plugin","tuple","native_config","client_config",*evidence}
    digest=re.compile(r"sha256:[a-f0-9]{64}")
    tuple_fields={"product_id","tree_digest","manifest_digest","distribution_id","distribution_kind","release_sequence","package_version","source_repository","source_revision","source_path","snapshot_sequence","snapshot_digest","binary_digest","dependency_identity","installer_version","adapter_version","client_version","os","architecture","observed_at"}
    tuple_digests={"tree_digest","manifest_digest","snapshot_digest","binary_digest"}
    def release_tuple(item,plugin):
        strings=tuple_fields-{"release_sequence","snapshot_sequence","client_version"}
        if (not isinstance(item,dict) or set(item)!=tuple_fields or item.get("product_id")!=plugin
            or any(type(item.get(field)) is not int or item[field]<1 for field in ("release_sequence","snapshot_sequence"))
            or any(type(item.get(field)) is not str or not item[field] for field in strings)
            or item.get("client_version") is not None
            or re.fullmatch(r"[a-f0-9]{40}",str(item.get("source_revision",""))) is None
            or any(digest.fullmatch(str(item.get(field,""))) is None for field in tuple_digests)
            or str(item.get("source_repository","")).startswith("/") or "//" in str(item.get("source_repository",""))
            or Path(str(item.get("source_path",""))).is_absolute() or ".." in Path(str(item.get("source_path",""))).parts):
            raise SystemExit(f"installed release tuple for {suffix}/{plugin} is invalid")
    if (not isinstance(value,dict) or set(value)!={"schema_version","client_id","entries"}
        or type(value.get("schema_version")) is not int or value["schema_version"]!=1
        or type(value.get("client_id")) is not str or value["client_id"]!=suffix
        or not isinstance(value.get("entries"),list) or not value["entries"]):
        raise SystemExit(f"installed native projection for {suffix} is invalid")
    heroes={"agent-code-navigator","context7","cloudflare-docs","chrome-devtools","notion"}
    plugins=set(); active_paths=set()
    for entry in value["entries"]:
        if (not isinstance(entry,dict) or set(entry)!=fields
            or type(entry.get("plugin")) is not str or not entry["plugin"] or entry["plugin"] in plugins
            or not isinstance(entry.get("tuple"),dict)
            or any(type(entry.get(field)) is not str or digest.fullmatch(entry[field]) is None for field in evidence)):
            raise SystemExit(f"installed native projection entry for {suffix} is invalid")
        release_tuple(entry["tuple"],entry["plugin"])
        plugins.add(entry["plugin"])
        client_config=entry["client_config"]; native_config=entry["native_config"]
        if (not isinstance(client_config,dict) or set(client_config)!={"path","sha256"}
            or not isinstance(native_config,dict) or set(native_config)!={"path","sha256"}
            or type(client_config.get("path")) is not str or type(native_config.get("path")) is not str
            or type(client_config.get("sha256")) is not str or digest.fullmatch(client_config["sha256"]) is None
            or type(native_config.get("sha256")) is not str or digest.fullmatch(native_config["sha256"]) is None
            or client_config["sha256"]!=native_config["sha256"]):
            raise SystemExit(f"installed native projection config for {suffix} is invalid")
        active=Path(client_config["path"]); native=Path(native_config["path"])
        try: active_relative=active.relative_to(profile)
        except ValueError: raise SystemExit(f"active native config {active} escapes profile")
        try: native_relative=native.relative_to(proof)
        except ValueError: raise SystemExit(f"native config proof {native} escapes proof hierarchy")
        if (not active_relative.parts or any(part in ("",".","..") for part in active_relative.parts)
            or native_relative.parts != ("native",f'{entry["plugin"]}.json')):
            raise SystemExit(f"installed native projection path for {suffix} is invalid")
        active_paths.add(active)
    duplicates=[group for group in ({path:[entry for entry in value["entries"] if Path(entry["client_config"]["path"])==path] for path in active_paths}).values() if len(group)>1]
    if duplicates:
        shared=profile / ".kiro" / "settings" / "mcp.json"
        if (suffix!="kiro" or active_paths!={shared} or len(duplicates)!=1 or len(duplicates[0])!=len(heroes)
            or len({entry["client_config"]["sha256"] for entry in duplicates[0]})!=1):
            raise SystemExit(f"installed native projection for {suffix} has conflicting active configs")
    if plugins != heroes:
        raise SystemExit(f"installed native projection for {suffix} is incomplete")
    return value

directory("/var/empty",0,0,0o755)
for suffix,(uid,gid) in zip(("codex","cursor","kiro","control"),identities):
    directory(f"/var/empty/uap-observer-{suffix}",uid,gid,0o700)
directory("/var/lib/uap-observer",0,0,0o711)
directory("/var/lib/uap-observer/state",observer_uid,observer_gid,0o700)
for name in ("jobs","workspaces","profiles","proofs"): directory(f"/var/lib/uap-observer/{name}",0,0,0o711)
for suffix,(uid,gid) in zip(("codex","cursor","kiro"),identities):
    profile=Path(f"/var/lib/uap-observer/profiles/{suffix}")
    directory(f"/var/lib/uap-observer/workspaces/{suffix}",uid,gid,0o700)
    proof=Path(f"/var/lib/uap-observer/proofs/{suffix}")
    if proof.exists():
        directory(str(profile),0,gid,0o510)
        directory(str(proof),0,gid,0o510)
        if {item.name for item in proof.iterdir()} != {"receipts.json","native-projection.json","native"}:
            raise SystemExit(f"installed proof inventory for {suffix} differs")
        directory(str(proof / "native"),0,gid,0o510)
        if {item.name for item in (proof / "native").iterdir()} != {f"{name}.json" for name in ("agent-code-navigator","context7","cloudflare-docs","chrome-devtools","notion")}:
            raise SystemExit(f"installed native proof inventory for {suffix} differs")
        for item in (proof / "receipts.json",proof / "native-projection.json",*(proof / "native").iterdir()):
            info=os.lstat(item)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_gid != gid or stat.S_IMODE(info.st_mode) != 0o440 or info.st_nlink != 1:
                raise SystemExit(f"installed proof file {item} differs")
        projection=native_projection((proof / "native-projection.json").read_bytes(),suffix,profile,proof)
        # Decode receipts with the same strict policy used by the projection.
        def strict_pairs(items):
            result={}; folded=set()
            for key,value in items:
                normalized=key.casefold()
                if key in result or normalized in folded: raise SystemExit(f"installed receipts for {suffix} are ambiguous")
                result[key]=value; folded.add(normalized)
            return result
        receipts=json.loads((proof / "receipts.json").read_bytes(),object_pairs_hook=strict_pairs,parse_constant=lambda value: (_ for _ in ()).throw(SystemExit(f"installed receipts for {suffix} contain a non-finite number")),parse_float=lambda value: float(value) if math.isfinite(float(value)) else (_ for _ in ()).throw(SystemExit(f"installed receipts for {suffix} contain a non-finite number")))
        evidence={"manager_add_sha256","manager_info_sha256","post_add_doctor_sha256"}; receipt_fields={"name","tuple",*evidence}
        records=receipts.get("receipts") if isinstance(receipts,dict) else None
        if (not isinstance(receipts,dict) or set(receipts)!={"schema_version","receipts"}
            or type(receipts.get("schema_version")) is not int or receipts["schema_version"]!=1
            or not isinstance(records,list) or len(records)!=len(heroes)
            or any(not isinstance(record,dict) or set(record)!=receipt_fields for record in records)):
            raise SystemExit(f"installed receipts for {suffix} are invalid")
        by_name={record.get("name"):record for record in records}
        if set(by_name)!=heroes or len(by_name)!=len(records): raise SystemExit(f"installed receipts for {suffix} are incomplete")
        for entry in projection["entries"]:
            record=by_name[entry["plugin"]]
            if record["tuple"]!=entry["tuple"] or any(record.get(field)!=entry.get(field) or digest.fullmatch(str(record.get(field,""))) is None for field in evidence):
                raise SystemExit(f"installed receipts for {suffix} do not bind projection evidence")
        for entry in projection["entries"]:
            active=Path(entry["client_config"]["path"])
            try: active.relative_to(profile)
            except ValueError: raise SystemExit(f"active native config {active} escapes profile")
            info=os.lstat(active)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_gid != gid or stat.S_IMODE(info.st_mode) != 0o440 or info.st_nlink != 1:
                raise SystemExit(f"active native config {active} is client-writable")
            parent=active.parent
            while True:
                directory(str(parent),0,gid,0o510)
                if parent == profile: break
                parent=parent.parent
            native=Path(entry["native_config"]["path"])
            native_info=os.lstat(native)
            if not stat.S_ISREG(native_info.st_mode) or native_info.st_uid != 0 or native_info.st_gid != gid or stat.S_IMODE(native_info.st_mode) != 0o440 or native_info.st_nlink != 1:
                raise SystemExit(f"native config proof {native} differs")
            active_fd=os.open(active,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW)
            native_fd=-1
            try:
                native_fd=os.open(native,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW)
                if os.fstat(active_fd)!=info or os.fstat(native_fd)!=native_info:
                    raise SystemExit(f"active native config {active} changed during verification")
                active_body=os.read(active_fd,(4 << 20)+1); native_body=os.read(native_fd,(4 << 20)+1)
            finally:
                if native_fd>=0: os.close(native_fd)
                os.close(active_fd)
            expected_digest=entry["client_config"]["sha256"]
            if (len(active_body)>4 << 20 or len(native_body)>4 << 20 or active_body!=native_body
                or "sha256:"+hashlib.sha256(active_body).hexdigest()!=expected_digest):
                raise SystemExit(f"active native config {active} differs from its protected proof")
    else:
        directory(str(profile),uid,gid,0o700)
directory("/var/lib/uap-observer-human",0,0,0o755)
directory("/var/lib/uap-observer-human/pending",0,identities[3][1],0o750)
for name in ("consumed","reserved"): directory(f"/var/lib/uap-observer-human/{name}",0,0,0o700)
config_gid=grp.getgrnam("uap-observer-adapter-config").gr_gid
directory("/var/lib/uap-observer-consent",0,0,0o755)
directory("/var/lib/uap-observer-consent/pending",0,config_gid,0o750)
for name in ("consumed","reserved"): directory(f"/var/lib/uap-observer-consent/{name}",0,0,0o700)
directory("/var/lib/caddy",caddy_uid,caddy_gid,0o700)
directory("/var/log/caddy",caddy_uid,caddy_gid,0o700)
PY
}

observer_validate_protected_inputs() {
  closure=$1
  observer_runtime=${2:-$closure/runtime}
  protected_root=${3:-/opt/uap-observer-inputs}
  PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$observer_runtime" observer_run_closure_python_script_neutral "$closure" python3 -B "$closure/etc/uap-observer-adapter-config.json" "$protected_root" <<'PY'
import sys
from pathlib import Path
from observer.fixed_runner import validate_adapter_input_access
validate_adapter_input_access(Path(sys.argv[1]),protected_root=Path(sys.argv[2]))
PY
}

observer_validate_installed_closure_sources() {
  closure=$1
  source_root=$2
  adapter_config=$3
  observer_config=$4
  caddy_config=$5
  egress_allowlist=$6
  runner_digest=$7
  adapter_digest=$8
  caddy_digest=$9
  observer_compare_regular_files_neutral "$observer_config" "$closure/etc/uap-observer.json"
  observer_compare_regular_files_neutral "$adapter_config" "$closure/etc/uap-observer-adapter-config.json"
  observer_compare_regular_files_neutral "$caddy_config" "$closure/etc/Caddyfile"
  observer_compare_regular_files_neutral "$egress_allowlist" "$closure/etc/uap-observer-egress-allowlist.json"
  test "$(observer_sha256_regular_neutral "$closure/libexec/uap-observer-runner")" = "$runner_digest"
  test "$(observer_sha256_regular_neutral "$closure/libexec/uap-observer-fixed-adapter")" = "$adapter_digest"
  observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-egress-proxy.py" "$closure/libexec/uap-observer-egress-proxy"
  test "$(observer_sha256_regular_neutral "$closure/bin/caddy")" = "$caddy_digest"
  for name in $(observer_directory_inventory_neutral "$source_root/observer"); do
    case "$name" in *.py) ;; *) continue ;; esac
    observer_compare_regular_files_neutral "$source_root/observer/$name" "$closure/runtime/observer/$name"
  done
  for name in $(observer_directory_inventory_neutral "$source_root/tests/e2e/schemas"); do
    case "$name" in *.schema.json) ;; *) continue ;; esac
    observer_compare_regular_files_neutral "$source_root/tests/e2e/schemas/$name" "$closure/runtime/tests/e2e/schemas/$name"
  done
  observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-signer.py" "$closure/runtime/uap-observer-signer.py"
  observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-attest-chatgpt.py" "$closure/libexec/uap-observer-attest-chatgpt"
  observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-attest-consent.py" "$closure/libexec/uap-observer-attest-consent"
  observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-provision-profile.py" "$closure/libexec/uap-observer-provision-profile"
  for unit in $observer_units; do observer_compare_regular_files_neutral "$source_root/deploy/$unit" "$closure/systemd/$unit"; done
  PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$closure/runtime" observer_run_closure_python_neutral "$closure" "$closure/venv/bin/python" -B -c 'import cryptography,jsonschema; import observer.http_server'
  PYTHONDONTWRITEBYTECODE=1 observer_run_closure_python_script_neutral "$closure" python3 -B "$closure/etc/uap-observer-adapter-config.json" "$closure/etc/uap-observer-adapters.json" "$adapter_digest" <<'PY'
import hashlib,json,math,os,sys
config_path,adapters_path,adapter_digest=sys.argv[1:]
def read(path):
    descriptor=os.open(path,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME)
    try:
        value=b""
        while True:
            block=os.read(descriptor,1<<20)
            if not block: return value
            value+=block
    finally: os.close(descriptor)
def strict_adapter_manifest(encoded,expected):
    def pairs(items):
        result={}; folded=set()
        for key,value in items:
            normalized=key.casefold()
            if key in result or normalized in folded:
                raise ValueError("installed adapter registry has duplicate or case-confusable members")
            result[key]=value; folded.add(normalized)
        return result
    def constant(value):
        raise ValueError(f"installed adapter registry has a non-finite number: {value}")
    def finite(value):
        decoded=float(value)
        if not math.isfinite(decoded):
            raise ValueError(f"installed adapter registry has a non-finite number: {value}")
        return decoded
    value=json.loads(encoded,object_pairs_hook=pairs,parse_constant=constant,parse_float=finite)
    if (not isinstance(value,dict) or set(value)!={"schema_version","config","artifacts"}
        or type(value.get("schema_version")) is not int or value["schema_version"]!=1
        or value!=expected):
        raise SystemExit("installed adapter registry differs")
    return value
config_digest="sha256:"+hashlib.sha256(read(config_path)).hexdigest()
artifacts={"runtime-attestations.json":"runtime","notion-oauth-attestations.json":"notion","chatgpt-cloudflare-attestation.json":"chatgpt","consent.json":"consent"}
expected={"schema_version":1,"config":{"path":"/opt/uap-observer-current/etc/uap-observer-adapter-config.json","sha256":config_digest},"artifacts":{artifact:{"path":f"/opt/uap-observer-current/libexec/uap-observer-adapter-{name}","sha256":"sha256:"+adapter_digest} for artifact,name in artifacts.items()}}
strict_adapter_manifest(read(adapters_path),expected)
PY
}

observer_sync_tree() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,stat,sys
directory_flags=os.O_RDONLY|os.O_CLOEXEC|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_NOATIME
file_flags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
root=os.open(sys.argv[1],directory_flags)
def sync_tree(directory: int) -> None:
    children=[]
    for name in os.listdir(directory):
        info=os.stat(name,dir_fd=directory,follow_symlinks=False)
        if stat.S_ISREG(info.st_mode):
            descriptor=os.open(name,file_flags,dir_fd=directory)
            try:
                if os.fstat(descriptor) != info: raise PermissionError("durability file changed during traversal")
                os.fsync(descriptor)
            finally: os.close(descriptor)
        elif stat.S_ISDIR(info.st_mode):
            child=os.open(name,directory_flags,dir_fd=directory)
            if os.fstat(child) != info:
                os.close(child); raise PermissionError("durability directory changed during traversal")
            children.append(child)
        elif not stat.S_ISLNK(info.st_mode):
            raise SystemExit("durability tree contains a special file")
    for child in children:
        try: sync_tree(child)
        finally: os.close(child)
    os.fsync(directory)
try: sync_tree(root)
finally: os.close(root)
PY
}

observer_sync_directory() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,sys
descriptor=os.open(sys.argv[1],os.O_RDONLY|os.O_CLOEXEC|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_NOATIME)
try: os.fsync(descriptor)
finally: os.close(descriptor)
PY
}

observer_cleanup_committed_stage_payload() {
  stage=$1
  for item in "$stage"/* "$stage"/.[!.]* "$stage"/..?*; do
    if [ ! -e "$item" ] && [ ! -L "$item" ]; then continue; fi
    case "${item##*/}" in
      journal-resolved) continue ;;
    esac
    rm -rf -- "$item" || return 1
  done
  observer_sync_directory "$stage" || return 1
}

observer_mark_recovery_resolved() {
  stage=$1
  outcome=$2
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$stage" "$outcome" <<'PY'
import errno,os,secrets,stat,sys
stage,outcome=sys.argv[1:]
if outcome not in ("current-v1","rollback-v1"): raise SystemExit("invalid recovery outcome")
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
directory=os.open(stage,flags)
temporary=f".journal-resolved-{os.getpid()}-{secrets.token_hex(16)}"
try:
    try:
        outcome_fd=os.open("recovery-outcome",os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=directory)
    except FileExistsError:
        outcome_fd=os.open("recovery-outcome",os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=directory)
        try:
            info=os.fstat(outcome_fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode)!=0o600 or info.st_nlink!=1 or os.read(outcome_fd,128)!=os.fsencode(outcome)+b"\n":
                raise PermissionError("recovery outcome differs")
        finally: os.close(outcome_fd)
    else:
        try:
            os.write(outcome_fd,os.fsencode(outcome)+b"\n"); os.fchown(outcome_fd,0,0); os.fchmod(outcome_fd,0o600); os.fsync(outcome_fd)
        finally: os.close(outcome_fd)
        os.fsync(directory)
    descriptor=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=directory)
    try:
        os.write(descriptor,b"resolved-v1\n")
        os.fchown(descriptor,0,0)
        os.fchmod(descriptor,0o600)
        os.fsync(descriptor)
    finally: os.close(descriptor)
    os.rename(temporary,"journal-resolved",src_dir_fd=directory,dst_dir_fd=directory)
    os.fsync(directory)
finally:
    try: os.unlink(temporary,dir_fd=directory)
    except FileNotFoundError: pass
    os.close(directory)
PY
}

apply_observer_closure_modes() {
  closure=$1
  config_group=${2:-uap-observer-adapter-config}
  caddy_group=${3:-caddy}
  chown -R root "$closure"
  find "$closure" -type d -exec chmod 0755 {} +
  find "$closure" -type f -perm /0111 -exec chmod 0755 {} +
  find "$closure" -type f ! -perm /0111 -exec chmod 0644 {} +
  chmod 0640 "$closure/etc/uap-observer-adapter-config.json" "$closure/etc/Caddyfile"
  chown "root:$config_group" "$closure/etc/uap-observer-adapter-config.json"
  chown "root:$caddy_group" "$closure/etc/Caddyfile"
  chmod 0755 "$closure/libexec/uap-observer-runner" "$closure/libexec/uap-observer-fixed-adapter"
  chmod 0755 "$closure/libexec/uap-observer-egress-proxy"
  for name in runtime notion chatgpt consent; do
    chmod 0755 "$closure/libexec/uap-observer-adapter-$name"
  done
  test "$(stat -c '%u:%g:%a' "$closure/etc/uap-observer.json")" = '0:0:644'
  test "$(stat -c '%u:%g:%a:%h' "$closure/etc/uap-observer-adapter-config.json")" = "0:$(getent group "$config_group" | cut -d: -f3):640:1"
  test "$(stat -c '%u:%g:%a:%h' "$closure/etc/Caddyfile")" = "0:$(getent group "$caddy_group" | cut -d: -f3):640:1"
  test "$(stat -c '%u:%g:%a' "$closure/etc/uap-observer-adapters.json")" = '0:0:644'
  test "$(stat -c '%u:%g:%a' "$closure/etc/uap-observer-egress-allowlist.json")" = '0:0:644'
  test "$(stat -c '%u:%g:%a' "$closure/libexec/uap-observer-egress-proxy")" = '0:0:755'
  test "$(stat -c '%u:%g:%a' "$closure/libexec/uap-observer-runner")" = '0:0:755'
  test -z "$(find "$closure" \( -type d -o -type f \) -perm /0022 -print -quit)"
}

observer_validate_systemd_topology() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" <<'PY'
import os,stat,sys
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
root_before=os.stat(sys.argv[1],follow_symlinks=False)
rootfd=os.open(sys.argv[1],flags)
info=os.fstat(rootfd)
if info!=root_before: raise PermissionError("systemd root changed while opening")
if (not stat.S_ISDIR(info.st_mode) or stat.S_ISLNK(info.st_mode)
        or info.st_uid != 0 or info.st_gid != 0 or info.st_mode & 0o022):
    raise SystemExit("systemd root is unsafe")
units=("uap-observer.service","uap-observer-signer.service","uap-observer-runner.service","uap-observer-runner.socket","uap-observer-caddy.service","uap-observer-egress-proxy.service","uap-observer-egress-proxy.socket")
dropins=("uap-observer.service.d","uap-observer-runner.service.d")
allowed=set(units+dropins)
actual={name for name in os.listdir(rootfd) if name.startswith("uap-observer")}
if actual - allowed: raise SystemExit("systemd observer inventory contains an unexpected target")

def validate_dropin(parentfd: int, name: str, before: os.stat_result) -> None:
    directory=os.open(name,flags,dir_fd=parentfd)
    try:
        info=os.fstat(directory)
        if info!=before: raise PermissionError("systemd drop-in changed while opening")
        for child_name in os.listdir(directory):
            child_info=os.stat(child_name,dir_fd=directory,follow_symlinks=False)
            if child_info.st_uid != 0 or child_info.st_gid != 0 or child_info.st_mode & 0o022:
                raise SystemExit("systemd drop-in is not root-controlled")
            if stat.S_ISDIR(child_info.st_mode):
                validate_dropin(directory,child_name,child_info)
            elif stat.S_ISREG(child_info.st_mode):
                if child_info.st_nlink != 1: raise SystemExit("systemd drop-in regular file has unsafe link count")
            else:
                raise SystemExit("systemd drop-in contains unsafe topology")
        if os.fstat(directory)!=info: raise PermissionError("systemd drop-in changed during traversal")
    finally: os.close(directory)

for name in units+dropins:
    try: item=os.stat(name,dir_fd=rootfd,follow_symlinks=False)
    except FileNotFoundError: continue
    if (item.st_uid != 0 or item.st_gid != 0
            or (not stat.S_ISLNK(item.st_mode) and item.st_mode & 0o022)):
        raise SystemExit("systemd target is not root-controlled")
    if name in units:
        if stat.S_ISREG(item.st_mode):
            if item.st_nlink != 1: raise SystemExit("systemd regular target has unsafe link count")
        elif not stat.S_ISLNK(item.st_mode):
            raise SystemExit("systemd unit target has unsafe type")
        continue
    if not stat.S_ISDIR(item.st_mode) or stat.S_ISLNK(item.st_mode):
        raise SystemExit("systemd drop-in directory is unsafe")
    validate_dropin(rootfd,name,item)
if os.fstat(rootfd)!=info: raise PermissionError("systemd root changed during traversal")
os.close(rootfd)
PY
}

observer_systemd_archive() {
  operation=$1
  backup=$2
  live=${3:-}
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$operation" "$backup" "$live" <<'PY'
import base64,ctypes,hashlib,json,math,os,secrets,stat,sys,tempfile

operation,backup_path,live_path=sys.argv[1:]
units=("uap-observer.service","uap-observer-signer.service","uap-observer-runner.service","uap-observer-runner.socket","uap-observer-caddy.service","uap-observer-egress-proxy.service","uap-observer-egress-proxy.socket")
dropins=("uap-observer.service.d","uap-observer-runner.service.d")
names=units+dropins
dirflags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
fileflags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
libc=ctypes.CDLL(None,use_errno=True)
libc.mount.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_char_p,ctypes.c_ulong,ctypes.c_void_p)
libc.umount2.argtypes=(ctypes.c_char_p,ctypes.c_int)
libc.readlinkat.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4; MS_NOEXEC=8
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18; MS_NOATIME=1024; MS_NODIRATIME=2048; MNT_DETACH=2
namespace_ready=False
def strict_json(encoded):
    def pairs(items):
        result={}; folded=set()
        for key,value in items:
            normalized=key.casefold()
            if key in result or normalized in folded: raise PermissionError("installer recovery journal is invalid")
            result[key]=value; folded.add(normalized)
        return result
    def constant(_value): raise PermissionError("installer recovery journal is invalid")
    def finite(value):
        decoded=float(value)
        if not math.isfinite(decoded): raise PermissionError("installer recovery journal is invalid")
        return decoded
    return json.loads(encoded,object_pairs_hook=pairs,parse_constant=constant,parse_float=finite)
def journal_identity(encoded):
    value=strict_json(encoded)
    if (not isinstance(value,dict) or set(value)!={"version","present","records"}
        or type(value.get("version")) is not int or value["version"]!=1
        or not isinstance(value.get("present"),list) or not isinstance(value.get("records"),list)
        or any(not isinstance(name,str) or name not in names for name in value["present"])
        or len(set(value["present"]))!=len(value["present"])
        or any(not isinstance(record,dict) for record in value["records"])):
        raise PermissionError("installer recovery journal is invalid")
    return value
def call(result,label):
    if result!=0:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
namespace_ready=True
def exact_link_metadata(value):
    return (value.st_dev,value.st_ino,stat.S_IFMT(value.st_mode),stat.S_IMODE(value.st_mode),value.st_uid,value.st_gid,value.st_nlink,value.st_atime_ns,value.st_mtime_ns,value.st_ctime_ns)

def open_directory(name,*,dir_fd=None):
    before=os.stat(name,dir_fd=dir_fd,follow_symlinks=False)
    descriptor=os.open(name,dirflags,dir_fd=dir_fd)
    if os.fstat(descriptor)!=before:
        os.close(descriptor); raise PermissionError("systemd directory changed while opening")
    return descriptor,before

def metadata(info):
    return {"type":stat.S_IFMT(info.st_mode),"mode":stat.S_IMODE(info.st_mode),"uid":info.st_uid,"gid":info.st_gid,"atime":info.st_atime_ns,"mtime":info.st_mtime_ns,"nlink":info.st_nlink}
def proc_link(parent,name): return f"/proc/self/fd/{parent}/{name}"
def xattrs(parent,name,info,descriptor=None):
    target=proc_link(parent,name) if stat.S_ISLNK(info.st_mode) else descriptor
    return [[base64.b64encode(os.fsencode(key)).decode(),base64.b64encode(os.getxattr(target,key,follow_symlinks=not stat.S_ISLNK(info.st_mode))).decode()] for key in sorted(os.listxattr(target,follow_symlinks=not stat.S_ISLNK(info.st_mode)))]
def set_xattrs(parent,name,info,values,descriptor=None):
    target=proc_link(parent,name) if stat.S_ISLNK(info.st_mode) else descriptor
    for encoded,value in values:
        key=os.fsdecode(base64.b64decode(encoded)); data=base64.b64decode(value)
        os.setxattr(target,key,data,follow_symlinks=not stat.S_ISLNK(info.st_mode))
def stable_link(parent,name,info):
    global namespace_ready
    if not namespace_ready:
        call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
        call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
        namespace_ready=True
    view=tempfile.mkdtemp(prefix="uap-observer-noatime-"); encoded=os.fsencode(view); mounted=False
    try:
        call(libc.mount(b"/proc/self/fd/"+str(parent).encode(),encoded,None,MS_BIND,None),"bind pinned systemd directory"); mounted=True
        call(libc.mount(None,encoded,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOATIME|MS_NODIRATIME,None),"remount systemd directory noatime")
        viewfd=os.open(view,dirflags)
        try:
            linkfd=os.open(name,os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=viewfd)
            try:
                if exact_link_metadata(os.fstat(linkfd))!=exact_link_metadata(info): raise PermissionError("systemd symlink changed while pinning")
                size=max(256,info.st_size+1)
                while True:
                    buffer=ctypes.create_string_buffer(size); length=libc.readlinkat(linkfd,b"",buffer,size)
                    if length<0:
                        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
                    if length<size: break
                    size*=2
                if exact_link_metadata(os.fstat(linkfd))!=exact_link_metadata(info): raise PermissionError("systemd symlink changed while reading")
                return os.fsdecode(buffer.raw[:length]),os.dup(linkfd)
            finally: os.close(linkfd)
        finally: os.close(viewfd)
    finally:
        if mounted: call(libc.umount2(encoded,MNT_DETACH),"unmount systemd noatime view")
        os.rmdir(view)
def trusted(info,link=False):
    if info.st_uid!=0 or info.st_gid!=0 or (not link and info.st_mode&0o022): raise PermissionError("systemd target is not root-controlled")

def scan(parent,name,prefix,*,allow_link=True):
    info=os.stat(name,dir_fd=parent,follow_symlinks=False); mode=info.st_mode
    if stat.S_ISREG(mode):
        trusted(info); descriptor=os.open(name,fileflags,dir_fd=parent)
        try:
            if os.fstat(descriptor)!=info: raise PermissionError("systemd file changed while reading")
            record=metadata(info); record["path"]=prefix
            attrs=xattrs(parent,name,info,descriptor)
            digest=hashlib.sha256()
            while True:
                block=os.read(descriptor,1<<20)
                if not block: break
                digest.update(block)
            record["payload"]=digest.hexdigest(); record["xattrs"]=attrs
            if os.fstat(descriptor)!=info or os.stat(name,dir_fd=parent,follow_symlinks=False)!=info or xattrs(parent,name,info,descriptor)!=attrs:
                raise PermissionError("systemd file changed while reading")
        finally: os.close(descriptor)
    elif stat.S_ISDIR(mode):
        trusted(info); descriptor,opened=open_directory(name,dir_fd=parent)
        try:
            if opened!=info: raise PermissionError("systemd directory changed while reading")
            record=metadata(info); record["path"]=prefix
            attrs=xattrs(parent,name,info,descriptor); record["xattrs"]=attrs
            children=[]
            for child in sorted(os.listdir(descriptor)):
                children.extend(scan(descriptor,child,prefix+"/"+child,allow_link=False))
            record["children"]=[child for child in sorted(os.listdir(descriptor))]
            if os.fstat(descriptor)!=info or os.stat(name,dir_fd=parent,follow_symlinks=False)!=info or xattrs(parent,name,info,descriptor)!=attrs: raise PermissionError("systemd directory changed while reading")
        finally: os.close(descriptor)
        return [record]+children
    elif stat.S_ISLNK(mode):
        if not allow_link: raise PermissionError("systemd drop-in contains a symlink")
        trusted(info,True)
        record=metadata(info); record["path"]=prefix
        record["payload"],pinned=stable_link(parent,name,info)
        try:
            attrs=xattrs(parent,name,info); record["xattrs"]=attrs
            after=os.stat(name,dir_fd=parent,follow_symlinks=False)
            if exact_link_metadata(os.fstat(pinned))!=exact_link_metadata(info) or exact_link_metadata(after)!=exact_link_metadata(info) or xattrs(parent,name,info)!=attrs: raise PermissionError("systemd symlink changed while reading")
        finally: os.close(pinned)
    else: raise PermissionError("systemd target has unsafe type")
    if stat.S_ISREG(mode) and info.st_nlink!=1: raise PermissionError("systemd regular target has unsafe link count")
    return [record]

def copy(parent,name,destination,dstname,prefix,allow_link=True):
    info=os.stat(name,dir_fd=parent,follow_symlinks=False); mode=info.st_mode
    if stat.S_ISREG(mode):
        trusted(info)
        if info.st_nlink!=1: raise PermissionError("systemd regular target has unsafe link count")
        source=os.open(name,fileflags,dir_fd=parent); target=os.open(dstname,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=destination)
        try:
            if os.fstat(source)!=info: raise PermissionError("systemd file changed while snapshotting")
            attributes=xattrs(parent,name,info,source)
            while True:
                block=os.read(source,1<<20)
                if not block: break
                view=memoryview(block)
                while view: view=view[os.write(target,view):]
            if os.fstat(source)!=info or os.stat(name,dir_fd=parent,follow_symlinks=False)!=info or xattrs(parent,name,info,source)!=attributes: raise PermissionError("systemd file changed while snapshotting")
            os.fchown(target,info.st_uid,info.st_gid); os.fchmod(target,stat.S_IMODE(mode)); set_xattrs(destination,dstname,info,attributes,target)
            os.utime(target,ns=(info.st_atime_ns,info.st_mtime_ns)); os.fsync(target)
        finally: os.close(target); os.close(source)
    elif stat.S_ISDIR(mode):
        trusted(info); source,opened=open_directory(name,dir_fd=parent); os.mkdir(dstname,0o700,dir_fd=destination); target,_=open_directory(dstname,dir_fd=destination)
        try:
            if opened!=info: raise PermissionError("systemd directory changed while snapshotting")
            attributes=xattrs(parent,name,info,source)
            for child in sorted(os.listdir(source)): copy(source,child,target,child,prefix+"/"+child,False)
            if os.fstat(source)!=info or os.stat(name,dir_fd=parent,follow_symlinks=False)!=info or xattrs(parent,name,info,source)!=attributes: raise PermissionError("systemd directory changed while snapshotting")
            os.fchown(target,info.st_uid,info.st_gid); os.fchmod(target,stat.S_IMODE(mode)); set_xattrs(destination,dstname,info,attributes,target)
            os.utime(target,ns=(info.st_atime_ns,info.st_mtime_ns)); os.fsync(target)
            if os.fstat(source)!=info or os.stat(name,dir_fd=parent,follow_symlinks=False)!=info or xattrs(parent,name,info,source)!=attributes: raise PermissionError("systemd directory changed while snapshotting")
        finally: os.close(target); os.close(source)
    elif stat.S_ISLNK(mode):
        if not allow_link: raise PermissionError("systemd drop-in contains a symlink")
        trusted(info,True); target_text,pinned=stable_link(parent,name,info)
        try:
            attributes=xattrs(parent,name,info)
            after=os.stat(name,dir_fd=parent,follow_symlinks=False)
            if exact_link_metadata(os.fstat(pinned))!=exact_link_metadata(info) or exact_link_metadata(after)!=exact_link_metadata(info) or xattrs(parent,name,info)!=attributes: raise PermissionError("systemd symlink changed while snapshotting")
        finally: os.close(pinned)
        os.symlink(target_text,dstname,dir_fd=destination)
        os.chown(dstname,info.st_uid,info.st_gid,dir_fd=destination,follow_symlinks=False); set_xattrs(destination,dstname,info,attributes)
        os.utime(dstname,ns=(info.st_atime_ns,info.st_mtime_ns),dir_fd=destination,follow_symlinks=False)
    else: raise PermissionError("systemd target has unsafe type")

def read_file(directory,name):
    descriptor=os.open(name,fileflags,dir_fd=directory)
    try:
        info=os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode)!=0o600 or info.st_nlink!=1: raise PermissionError("journal control file is unsafe")
        value=b""
        while True:
            block=os.read(descriptor,1<<20)
            if not block: break
            value+=block
        return value
    finally: os.close(descriptor)
def write_file(directory,name,data):
    descriptor=os.open(name,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=directory)
    try: os.write(descriptor,data); os.fchown(descriptor,0,0); os.fchmod(descriptor,0o600); os.fsync(descriptor)
    finally: os.close(descriptor)
def load_archive():
    backup,info=open_directory(backup_path)
    if info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode)!=0o700: raise PermissionError("journal root is unsafe")
    items,item_info=open_directory("items",dir_fd=backup)
    if item_info.st_uid or item_info.st_gid or stat.S_IMODE(item_info.st_mode)!=0o700: raise PermissionError("journal items are unsafe")
    manifest=read_file(backup,"manifest")
    identity=journal_identity(read_file(backup,"identity.json"))
    expected_manifest=b"".join((b"present " if name in identity["present"] else b"missing ")+str(index).encode()+b" "+name.encode()+b"\n" for index,name in enumerate(names))
    if manifest!=expected_manifest: raise PermissionError("installer recovery journal is invalid")
    return backup,items,identity,info,item_info

if operation=="create":
    parent_path=os.path.dirname(backup_path) or "."; leaf=os.path.basename(backup_path)
    parent,_=open_directory(parent_path); live,rootinfo=open_directory(live_path)
    created=False
    try:
        trusted(rootinfo)
        if not stat.S_ISDIR(rootinfo.st_mode): raise PermissionError("systemd root is unsafe")
        allowed=set(names); actual={name for name in os.listdir(live) if name.startswith("uap-observer")}
        if actual-allowed: raise PermissionError("systemd observer inventory contains an unexpected target")
        os.mkdir(leaf,0o700,dir_fd=parent); created=True; backup,_=open_directory(leaf,dir_fd=parent); os.mkdir("items",0o700,dir_fd=backup); items,_=open_directory("items",dir_fd=backup)
        try:
            present=[]; records=[]
            for index,name in enumerate(names):
                try: entry=os.stat(name,dir_fd=live,follow_symlinks=False)
                except FileNotFoundError: continue
                if name in units and not (stat.S_ISREG(entry.st_mode) or stat.S_ISLNK(entry.st_mode)): raise PermissionError("systemd unit target has unsafe type")
                if name in dropins and not stat.S_ISDIR(entry.st_mode): raise PermissionError("systemd drop-in target has unsafe type")
                allow_link=name in units
                copy(live,name,items,str(index),name,allow_link); present.append(name); records.extend(scan(live,name,name,allow_link=allow_link))
            if os.fstat(live)!=rootinfo: raise PermissionError("systemd root changed while snapshotting")
            identity={"version":1,"present":present,"records":records}
            copied=[]
            for index,name in enumerate(names):
                if name in present: copied.extend(scan(items,str(index),name,allow_link=name in units))
            if copied!=records: raise PermissionError("journal snapshot identity differs")
            encoded=json.dumps(identity,sort_keys=True,separators=(",",":")).encode()+b"\n"
            manifest=b"".join((b"present " if name in present else b"missing ")+str(index).encode()+b" "+name.encode()+b"\n" for index,name in enumerate(names))
            write_file(backup,"manifest",manifest); write_file(backup,"identity.json",encoded); os.fsync(items); os.fsync(backup)
        finally: os.close(items); os.close(backup)
    except BaseException:
        if created:
            # The exclusive journal is uncommitted; the shell removes only this exact name.
            os.close(live); os.close(parent); raise
        raise
    finally:
        try: os.close(live)
        except OSError: pass
        try: os.close(parent)
        except OSError: pass
elif operation=="manifest":
    backup,items,identity,backup_info,item_info=load_archive()
    try:
        os.write(1,read_file(backup,"manifest"))
        if os.fstat(backup)!=backup_info or os.fstat(items)!=item_info: raise PermissionError("journal directory changed while reading")
    finally: os.close(items); os.close(backup)
elif operation in ("validate","compare","compare-stable"):
    backup,items,identity,backup_info,item_info=load_archive()
    try:
        actual=[]
        if set(os.listdir(items))!={str(names.index(name)) for name in identity["present"]}: raise PermissionError("journal inventory differs")
        for index,name in enumerate(names):
            if name in identity["present"]: actual.extend(scan(items,str(index),name,allow_link=name in units))
        if actual!=identity["records"]: raise PermissionError("journal payload differs")
        if operation in ("compare","compare-stable"):
            live,live_info=open_directory(live_path)
            try:
                expected_inventory=set(identity["present"])
                actual_inventory={name for name in os.listdir(live) if name.startswith("uap-observer")}
                if actual_inventory!=expected_inventory: raise PermissionError("systemd observer inventory differs from journal")
                current=[]
                for name in names:
                    try: os.stat(name,dir_fd=live,follow_symlinks=False)
                    except FileNotFoundError:
                        if name in identity["present"]: raise PermissionError("systemd target disappeared after journaling")
                    else:
                        if name not in identity["present"]: raise PermissionError("systemd target appeared after journaling")
                        current.extend(scan(live,name,name,allow_link=name in units))
                if operation=="compare":
                    if current!=identity["records"]: raise PermissionError("systemd target drifted after journaling")
                else:
                    if len(current)!=len(identity["records"]): raise PermissionError("systemd target topology differs from journal")
                    for expected,observed in zip(identity["records"],current):
                        if expected.keys()!=observed.keys(): raise PermissionError("systemd target fields differ from journal")
                        for field,value in expected.items():
                            if field=="atime":
                                if observed[field] < value: raise PermissionError("systemd target atime moved backwards after reload")
                            elif observed[field]!=value:
                                raise PermissionError("systemd stable target field drifted after reload")
                if {name for name in os.listdir(live) if name.startswith("uap-observer")}!=expected_inventory:
                    raise PermissionError("systemd observer inventory changed while comparing journal")
                if os.fstat(live)!=live_info: raise PermissionError("systemd root changed while comparing")
            finally: os.close(live)
        if os.fstat(backup)!=backup_info or os.fstat(items)!=item_info: raise PermissionError("journal directory changed while validating")
    finally: os.close(items); os.close(backup)
else: raise SystemExit("invalid journal operation")
PY
  result=$?
  if [ "$operation" = create ] && [ "$result" -ne 0 ] && { [ -e "$backup" ] || [ -L "$backup" ]; }; then rm -rf -- "$backup"; fi
  return "$result"
}

journal_observer_systemd() { observer_systemd_archive create "$1" "$2"; }
validate_observer_systemd_journal() { observer_systemd_archive validate "$1"; }
observer_compare_systemd_journal() { observer_systemd_archive compare "$1" "$2"; }
observer_compare_systemd_journal_stable() { observer_systemd_archive compare-stable "$1" "$2"; }

observer_require_complete_systemd_inventory() {
  root=$1
  shift
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$root" "$@" <<'PY'
import os,stat,sys
path=sys.argv[1]; expected=set(sys.argv[2:])
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
before=os.stat(path,follow_symlinks=False); root=os.open(path,flags)
try:
    if os.fstat(root)!=before or not stat.S_ISDIR(before.st_mode): raise PermissionError("systemd root changed while binding inventory")
    actual={name for name in os.listdir(root) if name.startswith("uap-observer")}
    if actual!=expected: raise PermissionError("systemd observer inventory is not the exact expected set")
    if os.fstat(root)!=before: raise PermissionError("systemd root changed while binding inventory")
finally: os.close(root)
PY
}

# Install one reviewed systemd entry without ever opening the destination.
# The only names created are exclusive entries in the destination directory;
# renameat2 protects the displaced name and rename atomically replaces any
# destination symlink raced in after validation.
observer_replace_systemd_entries() {
  systemd_root=$1
  shift
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$systemd_root" "$@" <<'PY'
import base64,ctypes,errno,hashlib,json,math,os,secrets,signal,stat,sys,tempfile
from pathlib import Path

root_path=sys.argv[1]
pairs=sys.argv[2:]
if not pairs or len(pairs) % 2: raise SystemExit("invalid systemd replacement arguments")
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
source_parents={}
libc=ctypes.CDLL(None,use_errno=True)
renameat2=getattr(libc,"renameat2",None)
if renameat2 is None:
    raise SystemExit("renameat2 is required for safe systemd replacement")
renameat2.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_int,ctypes.c_char_p,ctypes.c_uint)
renameat2.restype=ctypes.c_int
libc.mount.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_char_p,ctypes.c_ulong,ctypes.c_void_p)
libc.umount2.argtypes=(ctypes.c_char_p,ctypes.c_int)
libc.readlinkat.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4; MS_NOEXEC=8
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18; MS_NOATIME=1024; MS_NODIRATIME=2048; MNT_DETACH=2
namespace_ready=False

# Use descriptor xattr operations for files/directories.  Linux has no fd for
# an unopened symlink, so its l*xattr operations use a stable /proc/self/fd
# parent and are bracketed by no-follow identity checks.  ENOTSUP while listing
# means that filesystem cannot store xattrs, and therefore has none to copy;
# any failure after a source xattr is observed is fatal.
flistxattr=libc.flistxattr; fgetxattr=libc.fgetxattr
fsetxattr=libc.fsetxattr; fremovexattr=libc.fremovexattr
llistxattr=libc.llistxattr; lgetxattr=libc.lgetxattr
lsetxattr=libc.lsetxattr; lremovexattr=libc.lremovexattr
flistxattr.argtypes=(ctypes.c_int,ctypes.c_void_p,ctypes.c_size_t)
fgetxattr.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
fsetxattr.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t,ctypes.c_int)
fremovexattr.argtypes=(ctypes.c_int,ctypes.c_char_p)
llistxattr.argtypes=(ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
lgetxattr.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
lsetxattr.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t,ctypes.c_int)
lremovexattr.argtypes=(ctypes.c_char_p,ctypes.c_char_p)
for function in (flistxattr,fgetxattr,llistxattr,lgetxattr): function.restype=ctypes.c_ssize_t
for function in (fsetxattr,fremovexattr,lsetxattr,lremovexattr): function.restype=ctypes.c_int

unsupported_xattr_errors={errno.ENOTSUP,errno.EOPNOTSUPP,errno.ENOSYS}

def xattr_names(function, target) -> list[bytes]:
    while True:
        size=function(target,None,0)
        if size < 0:
            value=ctypes.get_errno()
            if value in unsupported_xattr_errors: return []
            raise OSError(value,os.strerror(value))
        if size == 0: return []
        buffer=ctypes.create_string_buffer(size)
        result=function(target,buffer,size)
        if result >= 0: return buffer.raw[:result].split(b"\0")[:-1]
        value=ctypes.get_errno()
        if value != errno.ERANGE: raise OSError(value,os.strerror(value))

def xattr_value(function, target, name: bytes) -> bytes:
    while True:
        size=function(target,name,None,0)
        if size < 0:
            value=ctypes.get_errno(); raise OSError(value,os.strerror(value),os.fsdecode(name))
        buffer=ctypes.create_string_buffer(max(1,size))
        result=function(target,name,buffer,size)
        if result >= 0: return buffer.raw[:result]
        value=ctypes.get_errno()
        if value != errno.ERANGE: raise OSError(value,os.strerror(value),os.fsdecode(name))

def set_xattr(function, target, name: bytes, value: bytes) -> None:
    buffer=ctypes.create_string_buffer(value,max(1,len(value)))
    if function(target,name,buffer,len(value),0) != 0:
        error=ctypes.get_errno(); raise OSError(error,os.strerror(error),os.fsdecode(name))

def sync_xattrs(src_names,src_get,src_target,dst_names,dst_get,dst_set,dst_remove,dst_target) -> None:
    source={name:xattr_value(src_get,src_target,name) for name in xattr_names(src_names,src_target)}
    destination=set(xattr_names(dst_names,dst_target))
    for name in destination-source.keys():
        if dst_remove(dst_target,name) != 0:
            error=ctypes.get_errno(); raise OSError(error,os.strerror(error),os.fsdecode(name))
    for name,value in source.items(): set_xattr(dst_set,dst_target,name,value)
    copied={name:xattr_value(dst_get,dst_target,name) for name in xattr_names(dst_names,dst_target)}
    if copied != source: raise OSError("systemd metadata xattrs differ after copy")

def sync_xattrs_fd(srcfd: int, dstfd: int) -> None:
    sync_xattrs(flistxattr,fgetxattr,srcfd,flistxattr,fgetxattr,fsetxattr,fremovexattr,dstfd)

def link_path(parentfd: int, name: str) -> bytes:
    return b"/proc/self/fd/"+str(parentfd).encode("ascii")+b"/"+os.fsencode(name)

def same_entry(first: os.stat_result, second: os.stat_result) -> bool:
    return first.st_dev == second.st_dev and first.st_ino == second.st_ino and stat.S_IFMT(first.st_mode) == stat.S_IFMT(second.st_mode)

def exact_metadata(info: os.stat_result, copied: os.stat_result) -> bool:
    return (stat.S_IFMT(info.st_mode),stat.S_IMODE(info.st_mode),info.st_uid,info.st_gid,info.st_atime_ns,info.st_mtime_ns,info.st_nlink) == (stat.S_IFMT(copied.st_mode),stat.S_IMODE(copied.st_mode),copied.st_uid,copied.st_gid,copied.st_atime_ns,copied.st_mtime_ns,copied.st_nlink)

def exact_link_metadata(value: os.stat_result):
    return (value.st_dev,value.st_ino,stat.S_IFMT(value.st_mode),stat.S_IMODE(value.st_mode),value.st_uid,value.st_gid,value.st_nlink,value.st_atime_ns,value.st_mtime_ns,value.st_ctime_ns)

def mount_call(result,label):
    if result!=0:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
mount_call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
mount_call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
namespace_ready=True
root_lstat=os.stat(root_path,follow_symlinks=False)
rootfd=os.open(root_path,flags)
if os.fstat(rootfd)!=root_lstat: raise PermissionError("systemd root changed while opening")

def neutral_readlink(parentfd: int, name: str, info: os.stat_result) -> str:
    global namespace_ready
    if not namespace_ready:
        mount_call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
        mount_call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
        namespace_ready=True
    view=tempfile.mkdtemp(prefix="uap-observer-noatime-"); encoded=os.fsencode(view); mounted=False
    try:
        mount_call(libc.mount(b"/proc/self/fd/"+str(parentfd).encode(),encoded,None,MS_BIND,None),"bind pinned systemd directory"); mounted=True
        mount_call(libc.mount(None,encoded,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOATIME|MS_NODIRATIME,None),"remount systemd directory noatime")
        viewfd=os.open(view,flags)
        try:
            linkfd=os.open(name,os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=viewfd)
            try:
                if exact_link_metadata(os.fstat(linkfd))!=exact_link_metadata(info): raise PermissionError("systemd symlink changed while pinning")
                size=max(256,info.st_size+1)
                while True:
                    buffer=ctypes.create_string_buffer(size); length=libc.readlinkat(linkfd,b"",buffer,size)
                    if length<0:
                        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
                    if length<size: break
                    size*=2
                if exact_link_metadata(os.fstat(linkfd))!=exact_link_metadata(info): raise PermissionError("systemd symlink changed while reading")
                return os.fsdecode(buffer.raw[:length]),os.dup(linkfd)
            finally: os.close(linkfd)
        finally: os.close(viewfd)
    finally:
        if mounted: mount_call(libc.umount2(encoded,MNT_DETACH),"unmount systemd noatime view")
        os.rmdir(view)

def open_directory(name, *, dir_fd=None):
    before=os.stat(name,dir_fd=dir_fd,follow_symlinks=False)
    descriptor=os.open(name,flags,dir_fd=dir_fd)
    if os.fstat(descriptor)!=before:
        os.close(descriptor); raise PermissionError("systemd directory changed while opening")
    return descriptor,before

def sync_xattrs_link(srcfd: int, srcname: str, dstfd: int, dstname: str, info: os.stat_result) -> None:
    source=link_path(srcfd,srcname); destination=link_path(dstfd,dstname)
    before=os.stat(dstname,dir_fd=dstfd,follow_symlinks=False)
    if not same_entry(info,os.stat(srcname,dir_fd=srcfd,follow_symlinks=False)):
        raise PermissionError("systemd symlink source changed while copying")
    sync_xattrs(llistxattr,lgetxattr,source,llistxattr,lgetxattr,lsetxattr,lremovexattr,destination)
    if not same_entry(info,os.stat(srcname,dir_fd=srcfd,follow_symlinks=False)) or not same_entry(before,os.stat(dstname,dir_fd=dstfd,follow_symlinks=False)):
        raise PermissionError("systemd symlink changed while copying metadata")

def trusted(info: os.stat_result, *, link: bool = False) -> None:
    if info.st_uid != 0 or info.st_gid != 0 or (not link and info.st_mode & 0o022):
        raise PermissionError("systemd source is not root-controlled")

def exclusive_name(prefix: str) -> str:
    return f".{prefix}-{os.getpid()}-{secrets.token_hex(16)}"

def rename_noreplace(oldfd: int, old: str, newfd: int, new: str) -> None:
    mutation_boundary("before-rename")
    if renameat2(oldfd,os.fsencode(old),newfd,os.fsencode(new),1) != 0:
        value=ctypes.get_errno()
        raise OSError(value,os.strerror(value),old)
    mutation_boundary("after-rename")

def copy_entry(srcfd: int, srcname: str, dstfd: int, dstname: str) -> None:
    info=os.stat(srcname,dir_fd=srcfd,follow_symlinks=False)
    mode=info.st_mode
    if stat.S_ISREG(mode):
        trusted(info)
        if info.st_nlink != 1: raise PermissionError("systemd source has unsafe link count")
        infd=os.open(srcname,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=srcfd)
        mutation_boundary("before-tree-file-create")
        outfd=os.open(dstname,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=dstfd)
        mutation_boundary("after-tree-file-create")
        try:
            if os.fstat(infd) != info: raise PermissionError("systemd source changed while copying")
            while True:
                block=os.read(infd,1 << 20)
                if not block: break
                view=memoryview(block)
                while view: view=view[os.write(outfd,view):]
            os.fchown(outfd,info.st_uid,info.st_gid)
            os.fchmod(outfd,stat.S_IMODE(mode))
            sync_xattrs_fd(infd,outfd)
            os.utime(outfd,ns=(info.st_atime_ns,info.st_mtime_ns))
            mutation_boundary("before-tree-fsync"); os.fsync(outfd); mutation_boundary("after-tree-fsync")
            if not exact_metadata(info,os.fstat(outfd)): raise OSError("systemd file metadata differs after copy")
        finally:
            os.close(outfd); os.close(infd)
    elif stat.S_ISDIR(mode):
        trusted(info)
        mutation_boundary("before-tree-mkdir"); os.mkdir(dstname,0o700,dir_fd=dstfd); mutation_boundary("after-tree-mkdir")
        infd,opened=open_directory(srcname,dir_fd=srcfd)
        outfd,_=open_directory(dstname,dir_fd=dstfd)
        try:
            if opened != info: raise PermissionError("systemd source changed while copying")
            info=os.fstat(infd)
            for child in os.listdir(infd): copy_entry(infd,child,outfd,child)
            os.fchown(outfd,info.st_uid,info.st_gid)
            os.fchmod(outfd,stat.S_IMODE(mode))
            sync_xattrs_fd(infd,outfd)
            os.utime(outfd,ns=(info.st_atime_ns,info.st_mtime_ns))
            mutation_boundary("before-tree-fsync"); os.fsync(outfd); mutation_boundary("after-tree-fsync")
            if not exact_metadata(info,os.fstat(outfd)): raise OSError("systemd directory metadata differs after copy")
            if os.fstat(infd) != info: raise PermissionError("systemd source changed while copying")
        finally:
            os.close(outfd); os.close(infd)
    elif stat.S_ISLNK(mode):
        trusted(info,link=True)
        target_text,pinned=neutral_readlink(srcfd,srcname,info)
        try:
            mutation_boundary("before-tree-symlink"); os.symlink(target_text,dstname,dir_fd=dstfd); mutation_boundary("after-tree-symlink")
            os.chown(dstname,info.st_uid,info.st_gid,dir_fd=dstfd,follow_symlinks=False)
            sync_xattrs_link(srcfd,srcname,dstfd,dstname,info)
            if exact_link_metadata(os.fstat(pinned))!=exact_link_metadata(info): raise PermissionError("systemd symlink source changed while copying")
        finally: os.close(pinned)
        os.utime(dstname,ns=(info.st_atime_ns,info.st_mtime_ns),dir_fd=dstfd,follow_symlinks=False)
        if not exact_metadata(info,os.stat(dstname,dir_fd=dstfd,follow_symlinks=False)):
            raise OSError("systemd symlink metadata differs after copy")
    else:
        raise PermissionError("systemd source has unsafe type")

def remove_entry(parentfd: int, child: str) -> None:
    info=os.stat(child,dir_fd=parentfd,follow_symlinks=False)
    if stat.S_ISDIR(info.st_mode):
        childfd,opened=open_directory(child,dir_fd=parentfd)
        try:
            if opened!=info: raise PermissionError("systemd directory changed while removing")
            info=os.fstat(childfd)
            for nested in os.listdir(childfd): remove_entry(childfd,nested)
            if not same_entry(info,os.fstat(childfd)): raise PermissionError("systemd directory changed while removing")
        finally: os.close(childfd)
        mutation_boundary("before-tree-rmdir")
        os.rmdir(child,dir_fd=parentfd)
        mutation_boundary("after-tree-rmdir")
    else:
        mutation_boundary("before-tree-unlink")
        os.unlink(child,dir_fd=parentfd)
        mutation_boundary("after-tree-unlink")

def fingerprint(parentfd: int, name: str, *, allow_link=True):
    info=os.stat(name,dir_fd=parentfd,follow_symlinks=False)
    common=(stat.S_IFMT(info.st_mode),stat.S_IMODE(info.st_mode),info.st_uid,info.st_gid,info.st_atime_ns,info.st_mtime_ns,info.st_nlink)
    if stat.S_ISREG(info.st_mode):
        descriptor=os.open(name,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=parentfd)
        try:
            if os.fstat(descriptor)!=info: raise PermissionError("systemd file changed while binding")
            digest=hashlib.sha256()
            while True:
                block=os.read(descriptor,1<<20)
                if not block: break
                digest.update(block)
            attrs=tuple((key,xattr_value(fgetxattr,descriptor,key)) for key in sorted(xattr_names(flistxattr,descriptor)))
            if os.fstat(descriptor)!=info: raise PermissionError("systemd file changed while binding")
            return common,digest.digest(),attrs
        finally: os.close(descriptor)
    if stat.S_ISDIR(info.st_mode):
        descriptor,opened=open_directory(name,dir_fd=parentfd)
        try:
            if opened!=info: raise PermissionError("systemd directory changed while binding")
            attrs=tuple((key,xattr_value(fgetxattr,descriptor,key)) for key in sorted(xattr_names(flistxattr,descriptor)))
            children=tuple((child,fingerprint(descriptor,child,allow_link=False)) for child in sorted(os.listdir(descriptor)))
            if os.fstat(descriptor)!=info: raise PermissionError("systemd directory changed while binding")
            return common,attrs,children
        finally: os.close(descriptor)
    if stat.S_ISLNK(info.st_mode) and allow_link:
        target=link_path(parentfd,name)
        payload,pinned=neutral_readlink(parentfd,name,info)
        try:
            attrs=tuple((key,xattr_value(lgetxattr,target,key)) for key in sorted(xattr_names(llistxattr,target)))
            if exact_link_metadata(os.fstat(pinned))!=exact_link_metadata(info) or exact_link_metadata(os.stat(name,dir_fd=parentfd,follow_symlinks=False))!=exact_link_metadata(info): raise PermissionError("systemd symlink changed while fingerprinting")
            return common,payload,attrs
        finally: os.close(pinned)
    raise PermissionError("systemd topology changed while binding")

expected_records={}
expected_present=None
expected_inventory=None
expected_fingerprints={}
restore_mode=bool(os.environ.get("UAP_OBSERVER_RESTORE_BACKUP"))
unit_names=("uap-observer.service","uap-observer-signer.service","uap-observer-runner.service","uap-observer-runner.socket","uap-observer-caddy.service","uap-observer-egress-proxy.service","uap-observer-egress-proxy.socket")
dropin_names=("uap-observer.service.d","uap-observer-runner.service.d")
journal_names=unit_names+dropin_names

def strict_journal_identity(encoded):
    def pairs(items):
        result={}; folded=set()
        for key,value in items:
            normalized=key.casefold()
            if key in result or normalized in folded: raise PermissionError("installer recovery journal is invalid")
            result[key]=value; folded.add(normalized)
        return result
    def constant(_value): raise PermissionError("installer recovery journal is invalid")
    def finite(value):
        decoded=float(value)
        if not math.isfinite(decoded): raise PermissionError("installer recovery journal is invalid")
        return decoded
    value=json.loads(encoded,object_pairs_hook=pairs,parse_constant=constant,parse_float=finite)
    if (not isinstance(value,dict) or set(value)!={"version","present","records"}
        or type(value.get("version")) is not int or value["version"]!=1
        or not isinstance(value.get("present"),list) or not isinstance(value.get("records"),list)
        or any(not isinstance(name,str) or name not in journal_names for name in value["present"])
        or len(set(value["present"]))!=len(value["present"])
        or any(not isinstance(record,dict) for record in value["records"])):
        raise PermissionError("installer recovery journal is invalid")
    return value

def observer_inventory() -> set[str]:
    return {name for name in os.listdir(rootfd) if name.startswith("uap-observer")}

mutation_step=0
def mutation_boundary(name: str) -> None:
    global mutation_step
    mutation_step+=1
    if os.environ.get("UAP_OBSERVER_REPLACE_TRACE") == "1":
        print(f"uap-observer-replace-boundary {mutation_step} {name}",file=sys.stderr,flush=True)
    requested={item for item in os.environ.get("UAP_OBSERVER_REPLACE_FAILPOINT","").split(",") if item}
    fail_at=int(os.environ.get("UAP_OBSERVER_REPLACE_FAIL_AT") or 0)
    if name in requested or (fail_at and mutation_step==fail_at):
        raise SystemExit(f"observer replacement failpoint: {name}")

def require_inventory(expected: set[str], boundary: str) -> None:
    if observer_inventory()!=expected:
        raise PermissionError(f"systemd observer inventory changed before {boundary}")

def displacement_test_boundary() -> None:
    ready=os.environ.get("UAP_OBSERVER_TEST_DISPLACEMENT_READY_FD")
    resume=os.environ.get("UAP_OBSERVER_TEST_DISPLACEMENT_RESUME_FD")
    if ready is not None or resume is not None:
        if ready is None or resume is None:
            raise RuntimeError("incomplete displacement test synchronization")
        os.write(int(ready),b"1")
        if os.read(int(resume),1)!=b"1":
            raise RuntimeError("displacement test synchronization closed")
    elif os.environ.get("UAP_OBSERVER_TEST_STOP_AFTER_DISPLACEMENT")=="1":
        os.kill(os.getpid(),signal.SIGSTOP)

def journal_precondition() -> None:
    global expected_records,expected_present,expected_inventory,expected_fingerprints
    backup_path=os.environ.get("UAP_OBSERVER_COMPARE_BACKUP") or os.environ.get("UAP_OBSERVER_RESTORE_BACKUP")
    if not backup_path: return
    backup,_=open_directory(backup_path)
    try:
        control=os.open("identity.json",os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=backup)
        try:
            encoded=b""
            while True:
                block=os.read(control,1<<20)
                if not block: break
                encoded+=block
            identity=strict_journal_identity(encoded)
        finally: os.close(control)
    finally: os.close(backup)
    expected=identity["records"]
    present=set(identity["present"])
    if restore_mode:
        known=set(journal_names)
        actual=observer_inventory()
        if actual-known:
            raise PermissionError("systemd observer inventory contains an unexpected target before journal-backed restore")
        # Restore is also the recovery operation for a process killed at any
        # replacement boundary.  At entry, a journal name may therefore hold
        # the pre-install value, the reviewed value, a previously restored
        # value, or be absent after a durable displacement.  Bind the exact
        # current fingerprint now and require it at every following mutation;
        # the complete journal remains the sole authority for the end state.
        for name in journal_names:
            if name in actual:
                expected_fingerprints[name]=fingerprint(rootfd,name,allow_link=name in unit_names)
        expected_present=set(actual)
        expected_inventory=set(actual)
        require_inventory(expected_inventory,"journal-backed restore")
        return
    records=[]
    names=journal_names
    def attrs(parent,name,info,descriptor=None):
        if stat.S_ISLNK(info.st_mode):
            target=link_path(parent,name); names_=xattr_names(llistxattr,target); getter=lgetxattr
        else: target=descriptor; names_=xattr_names(flistxattr,target); getter=fgetxattr
        return [[base64.b64encode(key).decode(),base64.b64encode(xattr_value(getter,target,key)).decode()] for key in sorted(names_)]
    def visit(parent,name,path,allow_link=True,output=None):
        if output is None: output=records
        info=os.stat(name,dir_fd=parent,follow_symlinks=False); mode=info.st_mode
        if stat.S_ISREG(mode):
            descriptor=os.open(name,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=parent)
            try:
                if os.fstat(descriptor)!=info: raise PermissionError("systemd target changed before replacement")
                info=os.fstat(descriptor)
                record={"path":path,"type":stat.S_IFMT(info.st_mode),"mode":stat.S_IMODE(info.st_mode),"uid":info.st_uid,"gid":info.st_gid,"atime":info.st_atime_ns,"mtime":info.st_mtime_ns,"nlink":info.st_nlink}
                digest=hashlib.sha256()
                while True:
                    block=os.read(descriptor,1<<20)
                    if not block: break
                    digest.update(block)
                record["payload"]=digest.hexdigest(); record["xattrs"]=attrs(parent,name,info,descriptor)
            finally: os.close(descriptor)
        elif stat.S_ISDIR(mode):
            descriptor,opened=open_directory(name,dir_fd=parent)
            try:
                if opened!=info: raise PermissionError("systemd directory changed before replacement")
                info=os.fstat(descriptor)
                record={"path":path,"type":stat.S_IFMT(info.st_mode),"mode":stat.S_IMODE(info.st_mode),"uid":info.st_uid,"gid":info.st_gid,"atime":info.st_atime_ns,"mtime":info.st_mtime_ns,"nlink":info.st_nlink}
                children=sorted(os.listdir(descriptor)); record["children"]=children; record["xattrs"]=attrs(parent,name,info,descriptor)
                output.append(record)
                for child in children: visit(descriptor,child,path+"/"+child,False,output)
                if os.fstat(descriptor)!=info: raise PermissionError("systemd directory changed before replacement")
            finally: os.close(descriptor)
            return
        elif stat.S_ISLNK(mode) and allow_link:
            record={"path":path,"type":stat.S_IFMT(mode),"mode":stat.S_IMODE(mode),"uid":info.st_uid,"gid":info.st_gid,"atime":info.st_atime_ns,"mtime":info.st_mtime_ns,"nlink":info.st_nlink}
            record["payload"],pinned=neutral_readlink(parent,name,info)
            try:
                record["xattrs"]=attrs(parent,name,info)
                after=os.stat(name,dir_fd=parent,follow_symlinks=False)
                if exact_link_metadata(os.fstat(pinned))!=exact_link_metadata(info) or exact_link_metadata(after)!=exact_link_metadata(info): raise PermissionError("systemd symlink changed before replacement")
            finally: os.close(pinned)
        else: raise PermissionError("systemd topology changed before replacement")
        output.append(record)
    for name in names:
        try: os.stat(name,dir_fd=rootfd,follow_symlinks=False)
        except FileNotFoundError:
            if name in present: raise PermissionError("systemd target disappeared before replacement")
        else:
            if name not in present: raise PermissionError("systemd target appeared before replacement")
            visit(rootfd,name,name,name in unit_names)
    if records!=expected: raise PermissionError("systemd target drifted immediately before replacement")
    expected_present=present
    expected_inventory=set(present)
    if observer_inventory()!=expected_inventory: raise PermissionError("systemd observer inventory changed before replacement")
    expected_records={name:[record for record in expected if record["path"]==name or record["path"].startswith(name+"/")] for name in present}

    globals()["capture_visit"]=visit

def validate_capture(name: str, displaced: str) -> None:
    captured=[]
    capture_visit(rootfd,displaced,name,name in unit_names,captured)
    if captured!=expected_records[name]:
        raise PermissionError("systemd destination raced after validation")

def replace(source_arg: str, name: str) -> None:
    if not name or "/" in name or name in (".",".."):
        raise ValueError("invalid systemd destination name")
    temporary=exclusive_name("uap-observer-new")
    displaced=exclusive_name("uap-observer-old")
    created=False
    moved=False
    installed=False
    deletion_started=False
    displaced_destroyed=False
    baseline_missing=False
    baseline=None
    staged_fingerprint=None
    displaced_fingerprint=None
    try:
        if expected_present is None:
            try: baseline=fingerprint(rootfd,name)
            except FileNotFoundError: baseline_missing=True
        if source_arg != "-":
            source=Path(source_arg)
            parent=os.fspath(source.parent)
            source_parent=source_parents.get(parent)
            if source_parent is None:
                source_parent,_=open_directory(parent)
                source_parents[parent]=source_parent
            try:
                copy_entry(source_parent,source.name,rootfd,temporary)
                created=True
                staged_fingerprint=fingerprint(rootfd,temporary)
            except BaseException:
                # copy_entry can fail immediately after its exclusive create.
                # Record that private name so the ordinary safe rollback path
                # removes it instead of leaking an unjournaled partial tree.
                try:
                    staged_fingerprint=fingerprint(rootfd,temporary)
                    created=True
                except FileNotFoundError:
                    pass
                raise
        global precondition_done
        if not precondition_done:
            journal_precondition()
            precondition_done=True
        if expected_inventory is not None: require_inventory(expected_inventory,"displacement")
        mutation_boundary("before-displacement")
        if expected_present is not None and name in expected_present:
            displaced_fingerprint=expected_fingerprints.get(name)
            if displaced_fingerprint is None:
                displaced_fingerprint=fingerprint(rootfd,name)
            elif fingerprint(rootfd,name)!=displaced_fingerprint:
                raise PermissionError("systemd destination raced after validation")
            rename_noreplace(rootfd,name,rootfd,displaced)
            moved=True
            expected_inventory.discard(name)
            mutation_boundary("after-displacement")
            displacement_test_boundary()
            require_inventory(expected_inventory,"captured-entry validation")
            if restore_mode:
                if fingerprint(rootfd,displaced)!=displaced_fingerprint:
                    raise PermissionError("systemd destination drifted before journal-backed restore")
            else: validate_capture(name,displaced)
            require_inventory(expected_inventory,"installation")
        elif expected_present is not None and name not in expected_present:
            if os.path.lexists(link_path(rootfd,name)):
                raise PermissionError("systemd missing destination raced after validation")
            require_inventory(expected_inventory,"installation")
        else:
            if baseline_missing:
                if os.path.lexists(link_path(rootfd,name)):
                    raise PermissionError("systemd missing destination raced after validation")
            else:
                if fingerprint(rootfd,name)!=baseline:
                    raise PermissionError("systemd destination raced after validation")
                rename_noreplace(rootfd,name,rootfd,displaced)
                moved=True
                displaced_fingerprint=baseline
                expected_inventory.discard(name)
                mutation_boundary("after-displacement")
                displacement_test_boundary()
                require_inventory(expected_inventory,"captured-entry validation")
                if fingerprint(rootfd,displaced)!=baseline:
                    raise PermissionError("systemd destination raced after validation")
                require_inventory(expected_inventory,"installation")
        if created:
            if expected_inventory is not None: require_inventory(expected_inventory,"installation")
            rename_noreplace(rootfd,temporary,rootfd,name)
            created=False
            installed=True
            if expected_inventory is not None:
                expected_inventory.add(name)
                expected_fingerprints[name]=staged_fingerprint
                require_inventory(expected_inventory,"post-install durability")
            mutation_boundary("after-installation")
        mutation_boundary("before-install-directory-fsync")
        os.fsync(rootfd)
        mutation_boundary("after-install-directory-fsync")
        if moved:
            if expected_inventory is not None: require_inventory(expected_inventory,"displaced-tree deletion")
            mutation_boundary("before-displaced-tree-deletion")
            deletion_started=True
            remove_entry(rootfd,displaced)
            deletion_started=False
            displaced_destroyed=True
            moved=False
            if expected_inventory is not None and source_arg == "-": expected_inventory.discard(name)
            if expected_inventory is not None: require_inventory(expected_inventory,"deletion durability")
            mutation_boundary("after-displaced-tree-deletion")
            mutation_boundary("before-delete-directory-fsync")
            os.fsync(rootfd)
            mutation_boundary("after-delete-directory-fsync")
    except BaseException as original:
        # Once deletion of the displaced baseline has begun, rollback can no
        # longer be proven complete.  Preserve every remaining entry and let
        # the durable journal drive a later fail-closed recovery attempt.
        if deletion_started or displaced_destroyed:
            raise
        try:
            if installed:
                require_inventory(expected_inventory,"rollback installed-entry removal")
                if staged_fingerprint is None or fingerprint(rootfd,name)!=staged_fingerprint:
                    raise PermissionError("installed systemd entry drifted before rollback removal")
                mutation_boundary("before-rollback-installed-removal")
                require_inventory(expected_inventory,"rollback installed-entry removal")
                remove_entry(rootfd,name); installed=False
                expected_inventory.discard(name); expected_fingerprints.pop(name,None)
                require_inventory(expected_inventory,"rollback displaced-entry restore")
                mutation_boundary("after-rollback-installed-removal")
            if moved:
                require_inventory(expected_inventory,"rollback displaced-entry restore")
                if displaced_fingerprint is None or fingerprint(rootfd,displaced)!=displaced_fingerprint:
                    raise PermissionError("displaced systemd entry drifted before rollback restore")
                if not restore_mode and expected_records:
                    validate_capture(name,displaced)
                if os.path.lexists(link_path(rootfd,name)):
                    raise PermissionError("systemd destination reappeared before rollback restore")
                mutation_boundary("before-rollback-displaced-restore")
                require_inventory(expected_inventory,"rollback displaced-entry restore")
                rename_noreplace(rootfd,displaced,rootfd,name); moved=False
                expected_inventory.add(name); expected_fingerprints[name]=displaced_fingerprint
                require_inventory(expected_inventory,"completed rollback restore")
                mutation_boundary("after-rollback-displaced-restore")
            if created:
                if staged_fingerprint is None or fingerprint(rootfd,temporary)!=staged_fingerprint:
                    raise PermissionError("staged systemd entry drifted before rollback cleanup")
                mutation_boundary("before-rollback-staging-removal")
                remove_entry(rootfd,temporary)
                mutation_boundary("after-rollback-staging-removal")
        except BaseException as rollback_error:
            raise rollback_error from original
        raise

try:
    root_info=os.fstat(rootfd)
    trusted(root_info)
    if not stat.S_ISDIR(root_info.st_mode): raise PermissionError("systemd root is unsafe")
    stale=[name for name in os.listdir(rootfd) if name.startswith((".uap-observer-new-",".uap-observer-old-"))]
    for name in stale: remove_entry(rootfd,name)
    if stale:
        mutation_boundary("before-stale-directory-fsync")
        os.fsync(rootfd)
        mutation_boundary("after-stale-directory-fsync")
    precondition_done=False
    if not os.environ.get("UAP_OBSERVER_COMPARE_BACKUP") and not os.environ.get("UAP_OBSERVER_RESTORE_BACKUP"):
        expected_inventory=observer_inventory()
    for index in range(0,len(pairs),2):
        replace(pairs[index],pairs[index+1])
        if int(os.environ.get("UAP_OBSERVER_REPLACE_ENTRY_FAIL_AT") or 0)==index//2+1:
            raise SystemExit(1)
finally:
    for source_parent in source_parents.values(): os.close(source_parent)
    os.close(rootfd)
PY
}

restore_observer_systemd() {
  backup=$1
  systemd_root=$2
  installed_source=${3:-}
  if [ -n "$installed_source" ] && { [ ! -d "$installed_source" ] || [ -L "$installed_source" ]; }; then
    installed_source=
  fi
  validate_observer_systemd_journal "$backup" || return 1
  journal_manifest=$(observer_systemd_archive manifest "$backup") || return 1
  set -- "$systemd_root"
  while read -r state index relative; do
    if [ "$state" = present ]; then
      set -- "$@" "$backup/items/$index" "$relative"
    else
      set -- "$@" - "$relative"
    fi
  done <<EOF
$journal_manifest
EOF
  UAP_OBSERVER_RESTORE_BACKUP=$backup \
    UAP_OBSERVER_INSTALLED_SOURCE=$installed_source \
    UAP_OBSERVER_REPLACE_FAILPOINT= \
    observer_replace_systemd_entries "$@" || return 1
  observer_compare_systemd_journal "$backup" "$systemd_root"
}

observer_recovery_failpoint() {
  case ",${UAP_OBSERVER_RECOVERY_FAILPOINT:-}," in *,$1,*) return 1 ;; esac
}

observer_validate_resolved_recovery() {
  stage=$1 closures_root=$2 current_pointer=$3 systemd_root=$4
  for control in journal-committed closure-digest recovery-outcome journal-resolved; do
    test -f "$stage/$control" && test ! -L "$stage/$control" || return 1
    test "$(stat -c '%u:%g:%a:%h' "$stage/$control")" = 0:0:600:1 || return 1
  done
  test "$(observer_read_control_file "$stage/journal-committed")" = committed-v1 || return 1
  test "$(observer_read_control_file "$stage/journal-resolved")" = resolved-v1 || return 1
  recovered_digest=$(observer_read_control_file "$stage/closure-digest") || return 1
  printf '%s\n' "$recovered_digest" | grep -Eq '^[0-9a-f]{64}$' || return 1
  outcome=$(observer_read_control_file "$stage/recovery-outcome") || return 1
  validate_observer_systemd_journal "$stage/systemd-backup" || return 1
  test -d "$closures_root" && test ! -L "$closures_root" || return 1
  test "$(stat -c '%u:%g:%a' "$closures_root")" = 0:0:755 || return 1
  case "$outcome" in
    current-v1)
      test -L "$current_pointer" || return 1
      test "$(observer_read_symlink_neutral "$current_pointer")" = "uap-observer-closures/$recovered_digest" || return 1
      test "$(observer_directory_inventory_neutral "$closures_root")" = "$recovered_digest" || return 1
      closure="$closures_root/$recovered_digest"
      test -d "$closure" && test ! -L "$closure" || return 1
      test "$(observer_closure_identity "$closure")" = "$recovered_digest" || return 1
      observer_compare_systemd_trees "$closure/systemd" "$stage/systemd" || return 1
      validate_observer_systemd_inventory "$closure/systemd" "$systemd_root" || return 1
      ;;
    rollback-v1)
      test ! -e "$current_pointer" && test ! -L "$current_pointer" || return 1
      test -z "$(observer_directory_inventory_neutral "$closures_root")" || return 1
      observer_compare_systemd_journal_stable "$stage/systemd-backup" "$systemd_root" || return 1
      ;;
    *) return 1 ;;
  esac
}

# Convert the authoritative resolved journal into a fixed, authenticated
# sibling tombstone.  The directory rename is the commit point: after its
# parent is fsynced, a new installation may safely own the original stage
# name while cleanup of the old non-authoritative tree is retried.
observer_tombstone_resolved_journal() {
  stage=$1
  tombstone=${stage}.resolved-tombstone
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$stage" "$tombstone" <<'PY'
import ctypes,os,stat,sys
stage,tombstone=map(os.path.abspath,sys.argv[1:])
if os.path.dirname(stage)!=os.path.dirname(tombstone): raise ValueError("journal tombstone parent differs")
parent_path=os.path.dirname(stage); source=os.path.basename(stage); target=os.path.basename(tombstone)
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
libc=ctypes.CDLL(None,use_errno=True); libc.renameat2.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_int,ctypes.c_char_p,ctypes.c_uint)
parent=os.open(parent_path,flags)
try:
    directory=os.open(source,flags,dir_fd=parent)
    try:
        info=os.fstat(directory)
        if info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode)!=0o700: raise PermissionError("resolved journal is unsafe")
        try:
            marker=os.open("journal-tombstone",os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_CLOEXEC|os.O_NOFOLLOW,0o600,dir_fd=directory)
        except FileExistsError:
            # A rename that was not durable across a crash can leave the
            # prepared marker under the original authoritative name.  Accept
            # only the exact marker and retry the same no-replace rename.
            marker=os.open("journal-tombstone",os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=directory)
            try:
                mark=os.fstat(marker)
                if (not stat.S_ISREG(mark.st_mode) or mark.st_uid or mark.st_gid or stat.S_IMODE(mark.st_mode)!=0o600
                        or mark.st_nlink!=1 or os.read(marker,128)!=b"resolved-tombstone-v1\n"):
                    raise PermissionError("prepared journal tombstone marker is invalid")
            finally: os.close(marker)
        else:
            try:
                os.write(marker,b"resolved-tombstone-v1\n"); os.fchown(marker,0,0); os.fchmod(marker,0o600); os.fsync(marker)
            finally: os.close(marker)
            os.fsync(directory)
    finally: os.close(directory)
    if libc.renameat2(parent,os.fsencode(source),parent,os.fsencode(target),1)!=0:
        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
finally: os.close(parent)
PY
}

observer_cleanup_resolved_tombstone() {
  stage=$1
  tombstone=${stage}.resolved-tombstone
  if [ ! -e "$tombstone" ] && [ ! -L "$tombstone" ]; then return 0; fi
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$tombstone" <<'PY'
import os,stat,sys
path=os.path.abspath(sys.argv[1]); parent_path=os.path.dirname(path); leaf=os.path.basename(path)
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
parent=os.open(parent_path,flags); directory=os.open(leaf,flags,dir_fd=parent); deleted=0; visited=0
def safe(info):
    if info.st_uid or info.st_gid or (not stat.S_ISLNK(info.st_mode) and info.st_mode&0o022):
        raise PermissionError("journal tombstone child is unsafe")
def boundary():
    global deleted
    deleted+=1
    selected=os.environ.get("UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT")
    if selected and int(selected)==deleted: raise RuntimeError("tombstone child deletion failpoint")
def remove_tree(parentfd,name,depth=0):
    global visited
    if depth>128: raise RuntimeError("journal tombstone depth bound exceeded")
    visited+=1
    if visited>200000: raise RuntimeError("journal tombstone entry bound exceeded")
    info=os.stat(name,dir_fd=parentfd,follow_symlinks=False); safe(info)
    if stat.S_ISDIR(info.st_mode):
        child=os.open(name,flags,dir_fd=parentfd)
        try:
            if os.fstat(child)!=info: raise PermissionError("journal tombstone raced while opening")
            children=os.listdir(child)
            if len(children)>200000-visited: raise RuntimeError("journal tombstone entry bound exceeded")
            for entry in sorted(children): remove_tree(child,entry,depth+1)
            os.fsync(child)
        finally: os.close(child)
        os.rmdir(name,dir_fd=parentfd); os.fsync(parentfd); boundary()
    elif stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode):
        os.unlink(name,dir_fd=parentfd); os.fsync(parentfd); boundary()
    else: raise PermissionError("journal tombstone contains a special object")
try:
    info=os.fstat(directory)
    if info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode)!=0o700: raise PermissionError("journal tombstone is unsafe")
    root_entries=os.listdir(directory)
    if len(root_entries)>200000: raise RuntimeError("journal tombstone entry bound exceeded")
    entries=set(root_entries)
    if entries:
        marker=os.open("journal-tombstone",os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME,dir_fd=directory)
        try:
            mark=os.fstat(marker)
            if (not stat.S_ISREG(mark.st_mode) or mark.st_uid or mark.st_gid or stat.S_IMODE(mark.st_mode)!=0o600
                    or mark.st_nlink!=1 or os.read(marker,128)!=b"resolved-tombstone-v1\n"):
                raise PermissionError("journal tombstone marker is invalid")
        finally: os.close(marker)
        for name in sorted(entries-{"journal-tombstone"}): remove_tree(directory,name)
        os.unlink("journal-tombstone",dir_fd=directory); os.fsync(directory); boundary()
    # An empty directory is the sole valid state after durable marker removal.
    if os.listdir(directory): raise PermissionError("journal tombstone cleanup raced")
finally: os.close(directory)
os.rmdir(leaf,dir_fd=parent); os.fsync(parent); os.close(parent)
PY
}

recover_observer_install() {
  stage=$1
  closures_root=$2
  current_pointer=$3
  systemd_root=$4
  manager=$5
  cleanup_partials=${6:-observer_cleanup_recovery_partials}
  journal_committed=0
  journal_resolved=0
  recovering_current=0
  # Finish an older resolved cleanup first.  This fixed sibling never owns the
  # active stage name and therefore cannot conceal or block a legitimate new
  # journal at $stage.
  observer_cleanup_resolved_tombstone "$stage" || return 1
  if [ ! -e "$stage" ] && [ ! -L "$stage" ]; then
    # Retried even when a prior attempt removed the name but its parent fsync
    # failed.  This makes the removal durability step independently retryable.
    observer_recovery_failpoint before-removed-journal-parent-fsync || return 1
    observer_sync_directory "$(dirname "$stage")" || return 1
    observer_recovery_failpoint after-removed-journal-parent-fsync || return 1
    return
  fi
  if [ ! -d "$stage" ] || [ -L "$stage" ]; then
    echo "installer recovery journal is invalid" >&2
    return 1
  fi
  test "$(stat -c '%u:%g:%a' "$stage")" = 0:0:700 || {
    echo "installer recovery journal is invalid" >&2
    return 1
  }
  if [ -e "$stage/journal-resolved" ] || [ -L "$stage/journal-resolved" ]; then
    journal_committed=1
    journal_resolved=1
    observer_validate_resolved_recovery "$stage" "$closures_root" "$current_pointer" "$systemd_root" || return 1
  elif [ ! -e "$stage/journal-committed" ] && [ ! -L "$stage/journal-committed" ]; then
    # The atomic marker is the sole durable proof that mutation could begin.
    PYTHONDONTWRITEBYTECODE=1 python3 -B - "$stage" <<'PY' || return 1
import os,stat,sys
flags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
root=os.open(sys.argv[1],flags)
def validate(directory):
    for name in os.listdir(directory):
        info=os.stat(name,dir_fd=directory,follow_symlinks=False)
        if stat.S_ISDIR(info.st_mode):
            child=os.open(name,flags,dir_fd=directory)
            try:
                if os.fstat(child)!=info: raise PermissionError("pre-commit staging directory changed")
                validate(child)
            finally: os.close(child)
        elif not (stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode)):
            raise SystemExit("pre-commit staging tree contains an unsafe object")
try: validate(root)
finally: os.close(root)
PY
  else
    journal_committed=1
    test -f "$stage/journal-committed" && test ! -L "$stage/journal-committed" || return 1
    test "$(stat -c '%u:%g:%a:%h' "$stage/journal-committed")" = 0:0:600:1 || return 1
    test "$(observer_read_control_file "$stage/journal-committed")" = committed-v1 || return 1
    test -f "$stage/closure-digest" && test ! -L "$stage/closure-digest" || return 1
    test "$(stat -c '%u:%g:%a:%h' "$stage/closure-digest")" = 0:0:600:1 || return 1
    recovered_digest=$(observer_read_control_file "$stage/closure-digest")
    printf '%s\n' "$recovered_digest" | grep -Eq '^[0-9a-f]{64}$' || return 1
    validate_observer_systemd_journal "$stage/systemd-backup" || return 1
    test -d "$closures_root" && test ! -L "$closures_root" || return 1
    test "$(stat -c '%u:%g:%a' "$closures_root")" = 0:0:755 || return 1
    # A current pointer means activation crossed its commit point.  Recovery is
    # only permitted to accept the exact journaled closure in that case.
    if [ -e "$current_pointer" ] || [ -L "$current_pointer" ]; then
      recovering_current=1
      test -L "$current_pointer" || return 1
      test "$(observer_read_symlink_neutral "$current_pointer")" = "uap-observer-closures/$recovered_digest" || return 1
      test -d "$closures_root/$recovered_digest" || return 1
      test ! -L "$closures_root/$recovered_digest" || return 1
      test "$(observer_closure_identity "$closures_root/$recovered_digest")" = "$recovered_digest" || return 1
      observer_sync_directory "$(dirname "$current_pointer")" || return 1
      observer_compare_systemd_trees "$closures_root/$recovered_digest/systemd" "$stage/systemd" || return 1
      validate_observer_systemd_inventory \
        "$closures_root/$recovered_digest/systemd" "$systemd_root" || return 1
    else
      restore_observer_systemd "$stage/systemd-backup" "$systemd_root" "$stage/systemd" || return 1
      observer_sync_tree "$systemd_root" || return 1
      # Exact journal authority, including archived atime, is required before
      # the service manager is allowed to inspect the restored units.
      observer_compare_systemd_journal "$stage/systemd-backup" "$systemd_root" || return 1
      observer_recovery_failpoint before-rollback-daemon-reload || return 1
      "$manager" daemon-reload || return 1
      observer_recovery_failpoint after-rollback-daemon-reload || return 1
      observer_compare_systemd_journal_stable "$stage/systemd-backup" "$systemd_root" || return 1
      candidate="$closures_root/$recovered_digest"
      if [ -e "$candidate" ] || [ -L "$candidate" ]; then
        observer_recovery_failpoint before-rollback-closure-deletion || return 1
        rm -rf -- "$candidate" || return 1
        observer_recovery_failpoint after-rollback-closure-deletion || return 1
      fi
      observer_sync_tree "$closures_root" || return 1
      observer_recovery_failpoint after-rollback-closure-fsync || return 1
      observer_compare_systemd_journal_stable "$stage/systemd-backup" "$systemd_root" || return 1
    fi
    if [ "$recovering_current" -eq 1 ]; then
      test -L "$current_pointer" || return 1
      test "$(observer_read_symlink_neutral "$current_pointer")" = "uap-observer-closures/$recovered_digest" || return 1
      test -d "$closures_root/$recovered_digest" || return 1
      test ! -L "$closures_root/$recovered_digest" || return 1
      test "$(observer_closure_identity "$closures_root/$recovered_digest")" = "$recovered_digest" || return 1
      test "$(observer_directory_inventory_neutral "$closures_root")" = "$recovered_digest" || return 1
      observer_compare_systemd_trees "$closures_root/$recovered_digest/systemd" "$stage/systemd" || return 1
      validate_observer_systemd_inventory \
        "$closures_root/$recovered_digest/systemd" "$systemd_root" || return 1
    else
      test ! -e "$current_pointer" && test ! -L "$current_pointer" || return 1
      test -z "$(observer_directory_inventory_neutral "$closures_root")" || return 1
      observer_compare_systemd_journal_stable "$stage/systemd-backup" "$systemd_root" || return 1
    fi
    observer_recovery_failpoint before-journal-resolution || return 1
    if [ "$recovering_current" -eq 1 ]; then outcome=current-v1; else outcome=rollback-v1; fi
    observer_mark_recovery_resolved "$stage" "$outcome" || return 1
    observer_recovery_failpoint after-journal-resolution || return 1
    journal_resolved=1
  fi
  observer_recovery_failpoint before-partial-cleanup || return 1
  "$cleanup_partials" || return 1
  observer_recovery_failpoint after-partial-cleanup || return 1
  if [ "$journal_resolved" -eq 1 ]; then
    observer_recovery_failpoint before-rollback-data-deletion || return 1
    observer_recovery_failpoint after-rollback-data-deletion || return 1
  fi
  observer_recovery_failpoint before-journal-directory-deletion || return 1
  if [ "$journal_resolved" -eq 1 ]; then
    observer_sync_directory "$stage" || return 1
    # Revalidate immediately before changing which name is authoritative.
    observer_validate_resolved_recovery "$stage" "$closures_root" "$current_pointer" "$systemd_root" || return 1
    observer_tombstone_resolved_journal "$stage" || return 1
    observer_recovery_failpoint after-resolved-journal-rename || return 1
    observer_sync_directory "$(dirname "$stage")" || return 1
    observer_recovery_failpoint after-resolved-journal-parent-fsync || return 1
    observer_cleanup_resolved_tombstone "$stage" || return 1
  else
    rm -rf -- "$stage" || return 1
    observer_sync_directory "$(dirname "$stage")" || return 1
  fi
  observer_recovery_failpoint after-journal-directory-deletion || return 1
  observer_recovery_failpoint after-journal-parent-fsync || return 1
}

observer_install_failpoint() {
  observer_install_step=$((observer_install_step + 1))
  test "${UAP_OBSERVER_INSTALL_FAIL_AT:-}" != "$observer_install_step"
}

observer_copy_systemd_tree_neutral() {
  staged=$1
  destination=$2
  set -- "$destination"
  for unit in $observer_units; do set -- "$@" "$staged/$unit" "$unit"; done
  for service in uap-observer uap-observer-runner; do
    set -- "$@" "$staged/$service.service.d" "$service.service.d"
  done
  UAP_OBSERVER_COMPARE_BACKUP= UAP_OBSERVER_REPLACE_FAILPOINT= UAP_OBSERVER_REPLACE_ENTRY_FAIL_AT= \
    observer_replace_systemd_entries "$@" || return 1
  validate_observer_systemd_inventory "$staged" "$destination"
}

activate_observer_systemd() {
  staged=$1
  systemd_root=$2
  backup=${3:-}
  observer_install_step=0
  if [ -n "$backup" ]; then
    validate_observer_systemd_journal "$backup" || return 1
    observer_compare_systemd_journal "$backup" "$systemd_root" || return 1
  fi
  set -- "$systemd_root"
  for unit in $observer_units; do set -- "$@" "$staged/$unit" "$unit"; done
  for service in uap-observer uap-observer-runner; do
    set -- "$@" "$staged/$service.service.d" "$service.service.d"
  done
  # This second descriptor-only identity check is deliberately adjacent to
  # the replacement process, after all validation that could consume atime.
  if [ -n "$backup" ]; then observer_compare_systemd_journal "$backup" "$systemd_root" || return 1; fi
  UAP_OBSERVER_COMPARE_BACKUP=$backup \
    UAP_OBSERVER_REPLACE_ENTRY_FAIL_AT=${UAP_OBSERVER_INSTALL_FAIL_AT:-} \
    observer_replace_systemd_entries "$@" || return 1
  # Keep the outer transaction checkpoint aligned with the exact replacement
  # inventory (seven units plus two drop-in directories).
  observer_install_step=0
  for unit in $observer_units; do observer_install_step=$((observer_install_step + 1)); done
  for service in uap-observer uap-observer-runner; do observer_install_step=$((observer_install_step + 1)); done
  validate_observer_systemd_inventory "$staged" "$systemd_root"
}

validate_observer_systemd_inventory() {
  reviewed=$1
  systemd_root=$2
  observer_validate_systemd_topology "$systemd_root" || return 1
  expected=$(printf '%s\n' $observer_units uap-observer.service.d uap-observer-runner.service.d | LC_ALL=C sort)
  actual=$(observer_directory_inventory_neutral "$systemd_root" uap-observer)
  test "$actual" = "$expected" || return 1
  observer_compare_systemd_trees "$reviewed" "$systemd_root"
}

observer_compare_systemd_trees() {
  PYTHONDONTWRITEBYTECODE=1 python3 -B - "$1" "$2" <<'PY'
import ctypes,os,signal,stat,sys,tempfile
names=("uap-observer.service","uap-observer-signer.service","uap-observer-runner.service","uap-observer-runner.socket","uap-observer-caddy.service","uap-observer-egress-proxy.service","uap-observer-egress-proxy.socket","uap-observer.service.d","uap-observer-runner.service.d")
dirflags=os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
fileflags=os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NOATIME
libc=ctypes.CDLL(None,use_errno=True)
libc.mount.argtypes=(ctypes.c_char_p,ctypes.c_char_p,ctypes.c_char_p,ctypes.c_ulong,ctypes.c_void_p)
libc.umount2.argtypes=(ctypes.c_char_p,ctypes.c_int)
libc.readlinkat.argtypes=(ctypes.c_int,ctypes.c_char_p,ctypes.c_void_p,ctypes.c_size_t)
CLONE_NEWNS=0x00020000; MS_RDONLY=1; MS_NOSUID=2; MS_NODEV=4; MS_NOEXEC=8
MS_REMOUNT=32; MS_BIND=4096; MS_REC=16384; MS_PRIVATE=1<<18; MS_NOATIME=1024; MS_NODIRATIME=2048; MNT_DETACH=2
namespace_ready=False
systemd_compare_ready_fd=os.environ.get("UAP_OBSERVER_TEST_SYSTEMD_COMPARE_READY_FD")
systemd_compare_resume_fd=os.environ.get("UAP_OBSERVER_TEST_SYSTEMD_COMPARE_RESUME_FD")
if (systemd_compare_ready_fd is None)!=(systemd_compare_resume_fd is None):
    raise RuntimeError("incomplete systemd comparison test synchronization")
def metadata(info):
    # Access time is deliberately not a cross-tree authority.  A successful
    # daemon-reload may legitimately read the live unit after installation,
    # while content, ownership, mode, mtime, link count and xattrs remain the
    # authenticated security state.  Journal restore uses its separate exact
    # archive comparison, which continues to include captured atime.
    return (stat.S_IFMT(info.st_mode),stat.S_IMODE(info.st_mode),info.st_uid,info.st_gid,info.st_mtime_ns,info.st_nlink)
def link_metadata(value): return (value.st_dev,value.st_ino)+metadata(value)+(value.st_ctime_ns,)
def call(result,label):
    if result!=0:
        error=ctypes.get_errno(); raise OSError(error,f"{label}: {os.strerror(error)}")
def attributes(parent,name,info,descriptor=None):
    target=f"/proc/self/fd/{parent}/{name}" if stat.S_ISLNK(info.st_mode) else descriptor
    follow=not stat.S_ISLNK(info.st_mode)
    return {key:os.getxattr(target,key,follow_symlinks=follow) for key in os.listxattr(target,follow_symlinks=follow)}
def race_point(name):
    if systemd_compare_ready_fd is not None and name==os.environ.get("UAP_OBSERVER_TEST_SYSTEMD_COMPARE_NAME"):
        os.write(int(systemd_compare_ready_fd),b"1")
        if os.read(int(systemd_compare_resume_fd),1)!=b"1":
            raise RuntimeError("systemd comparison test synchronization closed")
def link(parent,name,info):
    global namespace_ready
    if not namespace_ready:
        call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
        call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
        namespace_ready=True
    view=tempfile.mkdtemp(prefix="uap-observer-noatime-"); encoded=os.fsencode(view); mounted=False
    try:
        call(libc.mount(b"/proc/self/fd/"+str(parent).encode(),encoded,None,MS_BIND,None),"bind pinned comparison directory"); mounted=True
        call(libc.mount(None,encoded,None,MS_REMOUNT|MS_BIND|MS_RDONLY|MS_NOATIME|MS_NODIRATIME,None),"remount comparison directory noatime")
        viewfd=os.open(view,dirflags)
        try:
            descriptor=os.open(name,os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=viewfd)
            try:
                if link_metadata(os.fstat(descriptor))!=link_metadata(info): raise PermissionError("systemd symlink changed while pinning comparison")
                size=max(256,info.st_size+1)
                while True:
                    buffer=ctypes.create_string_buffer(size); length=libc.readlinkat(descriptor,b"",buffer,size)
                    if length<0:
                        error=ctypes.get_errno(); raise OSError(error,os.strerror(error))
                    if length<size: break
                    size*=2
                if link_metadata(os.fstat(descriptor))!=link_metadata(info): raise PermissionError("systemd symlink changed during comparison")
                return os.fsdecode(buffer.raw[:length]),os.dup(descriptor)
            finally: os.close(descriptor)
        finally: os.close(viewfd)
    finally:
        if mounted: call(libc.umount2(encoded,MNT_DETACH),"unmount comparison noatime view")
        os.rmdir(view)
def compare(first,name,second):
    left=os.stat(name,dir_fd=first,follow_symlinks=False); right=os.stat(name,dir_fd=second,follow_symlinks=False)
    if metadata(left)!=metadata(right): raise SystemExit("systemd metadata differs")
    if stat.S_ISREG(left.st_mode):
        a=os.open(name,fileflags,dir_fd=first); b=os.open(name,fileflags,dir_fd=second)
        try:
            if os.fstat(a)!=left or os.fstat(b)!=right: raise PermissionError("systemd file changed during comparison")
            left_attrs=attributes(first,name,left,a); right_attrs=attributes(second,name,right,b)
            if left_attrs!=right_attrs: raise SystemExit("systemd xattrs differ")
            while True:
                one=os.read(a,1<<20); two=os.read(b,1<<20)
                if one!=two: raise SystemExit("systemd payload differs")
                if not one: break
            race_point(name)
            if (os.fstat(a)!=left or os.fstat(b)!=right
                    or os.stat(name,dir_fd=first,follow_symlinks=False)!=left
                    or os.stat(name,dir_fd=second,follow_symlinks=False)!=right):
                raise PermissionError("systemd file changed during comparison")
            if attributes(first,name,left,a)!=left_attrs or attributes(second,name,right,b)!=right_attrs:
                raise PermissionError("systemd file attributes changed during comparison")
        finally: os.close(b); os.close(a)
    elif stat.S_ISDIR(left.st_mode):
        a=os.open(name,dirflags,dir_fd=first); b=os.open(name,dirflags,dir_fd=second)
        try:
            if os.fstat(a)!=left or os.fstat(b)!=right: raise PermissionError("systemd directory changed during comparison")
            left_attrs=attributes(first,name,left,a); right_attrs=attributes(second,name,right,b)
            if left_attrs!=right_attrs: raise SystemExit("systemd xattrs differ")
            children=sorted(os.listdir(a))
            if children!=sorted(os.listdir(b)): raise SystemExit("systemd topology differs")
            for child in children: compare(a,child,b)
            if (os.fstat(a)!=left or os.fstat(b)!=right
                    or os.stat(name,dir_fd=first,follow_symlinks=False)!=left
                    or os.stat(name,dir_fd=second,follow_symlinks=False)!=right): raise PermissionError("systemd directory changed during comparison")
            if attributes(first,name,left,a)!=left_attrs or attributes(second,name,right,b)!=right_attrs:
                raise PermissionError("systemd directory attributes changed during comparison")
        finally: os.close(b); os.close(a)
    elif stat.S_ISLNK(left.st_mode):
        left_target,left_pin=link(first,name,left); right_target,right_pin=link(second,name,right)
        try:
            left_attrs=attributes(first,name,left); right_attrs=attributes(second,name,right)
            if left_target!=right_target or left_attrs!=right_attrs: raise SystemExit("systemd symlink differs")
            if link_metadata(os.fstat(left_pin))!=link_metadata(left) or link_metadata(os.fstat(right_pin))!=link_metadata(right): raise PermissionError("systemd symlink changed during comparison")
            if link_metadata(os.stat(name,dir_fd=first,follow_symlinks=False))!=link_metadata(left) or link_metadata(os.stat(name,dir_fd=second,follow_symlinks=False))!=link_metadata(right): raise PermissionError("systemd symlink name changed during comparison")
            if attributes(first,name,left)!=left_attrs or attributes(second,name,right)!=right_attrs: raise PermissionError("systemd symlink attributes changed during comparison")
        finally: os.close(right_pin); os.close(left_pin)
    else: raise SystemExit("systemd type differs")
call(libc.unshare(CLONE_NEWNS),"private mount namespace unavailable")
call(libc.mount(None,b"/",None,MS_REC|MS_PRIVATE,None),"make mounts private")
namespace_ready=True
first_before=os.stat(sys.argv[1],follow_symlinks=False); second_before=os.stat(sys.argv[2],follow_symlinks=False)
first=os.open(sys.argv[1],dirflags); second=os.open(sys.argv[2],dirflags)
try:
    if os.fstat(first)!=first_before or os.fstat(second)!=second_before: raise PermissionError("systemd root changed during comparison")
    for name in names: compare(first,name,second)
    if os.fstat(first)!=first_before or os.fstat(second)!=second_before: raise PermissionError("systemd root changed during comparison")
finally: os.close(second); os.close(first)
PY
}

reload_observer_systemd() {
  manager=$1
  case ",${UAP_OBSERVER_INSTALL_FAILPOINT:-}," in *,before-daemon-reload,*) return 1 ;; esac
  "$manager" daemon-reload || return 1
  case ",${UAP_OBSERVER_INSTALL_FAILPOINT:-}," in *,after-daemon-reload,*) return 1 ;; esac
  observer_install_failpoint || return 1
}
