import zlib, struct
def rawfile(name, data):
    comp = zlib.compress(data, 9)
    return struct.pack('>IIII', 0xffffffff, len(comp), len(data), 0xffffffff) + name.encode() + b'\0' + comp
def build(files, out):
    body = b''.join(rawfile(n, d) for n, d in files)
    n = len(files)
    lst = struct.pack('>IIII', 0, 0, n, 0xffffffff) + struct.pack('>I', 0) + b''.join(struct.pack('>II', 40, 0xffffffff) for _ in files)
    blocks = [0x40 + 8*n, 0, 0, len(body) + 0x80, 0, 0, 0]
    dec_nohdr = lst + body
    hdr = struct.pack('>II', len(dec_nohdr) + 36 - 36 + 0, 0)  # fixed below
    xf = struct.pack('>II', 0, 0) + b''.join(struct.pack('>I', x) for x in blocks) + dec_nohdr
    xf = struct.pack('>I', len(xf) - 36) + xf[4:]
    z = zlib.compress(xf, 1)
    total = 40 + len(z)
    head = b'IWffu100' + struct.pack('>IIII', 0x4f2, 0x404, 0x01ce4854, 0xd9e50240) + struct.pack('>IIII', 1, 0, total, total)
    open(out, 'wb').write(head + z)
    return total
cfg = b'// MW3 co-op patch v0.0.50 - S1 test patch ff v01\r\nseta cg_fov 85\r\n'
print(build([('autoexec_dev.cfg', cfg)], 'patch_fusion.ff'))
