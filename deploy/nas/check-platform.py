"""Read-only host/ELF architecture preflight; never starts a service."""
import pathlib,platform,struct,sys

def architecture(machine):
    value={'x86_64':'amd64','amd64':'amd64','aarch64':'arm64','arm64':'arm64'}.get(machine.lower())
    if value is None:raise ValueError('Supported hosts: Linux amd64 or arm64')
    return value

def check_binary(path,arch):
    with pathlib.Path(path).open('rb') as stream:
        header=stream.read(64)
        if len(header)!=64 or header[:6]!=b'\x7fELF\x02\x01':raise ValueError('Expected a 64-bit little-endian Linux ELF binary')
        kind,machine=struct.unpack_from('<HH',header,16)
        if kind not in (2,3) or machine!={'amd64':62,'arm64':183}[arch]:raise ValueError('Binary architecture does not match this host')
        offset=struct.unpack_from('<Q',header,32)[0];entry,count=struct.unpack_from('<HH',header,54)
        if entry<56 or count>4096:raise ValueError('Invalid ELF program headers')
        stream.seek(offset)
        for _ in range(count):
            block=stream.read(entry)
            if len(block)!=entry:raise ValueError('Truncated ELF program headers')
            if struct.unpack_from('<I',block)[0]==3:raise ValueError('Expected a standalone binary without a dynamic interpreter')

if __name__=='__main__':
    try:
        if platform.system()!='Linux':raise ValueError('This installer targets Linux only')
        arch=architecture(platform.machine());check_binary(sys.argv[1],arch)
        print('Platform preflight passed: Linux '+arch)
    except (ValueError,OSError,IndexError) as error:
        print(str(error),file=sys.stderr);sys.exit(1)
