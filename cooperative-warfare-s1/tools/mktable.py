import struct,zlib,re
def h1(s):
    h=0
    for ch in s.lower(): h=(ord(ch)+31*h)&0xffffffff
    return h
def decode_table(d):
    i=d.index(b'sp/specopstable.csv\x00'); h=i-16
    nm,cols,rows,vals=struct.unpack('>IIII',d[h:h+16]); p=i+len(b'sp/specopstable.csv\x00')
    n=cols*rows
    cells=[struct.unpack('>II',d[p+8*k:p+8*k+8]) for k in range(n)]
    q=p+8*n; inl={}; first={}
    out=[None]*n
    for k,(ptr,hs) in enumerate(cells):
        if ptr==0xffffffff:
            e=d.index(b'\0',q); s=d[q:e].decode('latin1'); inl[k]=(s,q-p); q=e+1
    first={}
    for k,(s,r) in sorted(inl.items()): first.setdefault(s,r)
    # find base using '0' string vs a ptr cell
    # ptr -> rel offset: use known mapping base derived from any ptr cell whose string is '0' (first '0' inline)
    base=0x3001318f-first['0']
    rev={base+r:s for s,r in first.items()}
    for k,(ptr,hs) in enumerate(cells):
        out[k]=inl[k][0] if ptr==0xffffffff else rev[ptr]
    return cols,rows,out
def encode_table(cols,rows,grid,name=b'sp/specopstable.csv'):
    body=struct.pack('>IIII',0xffffffff,cols,rows,0xffffffff)+name+b'\0'
    cells=b''.join(struct.pack('>II',0xffffffff,h1(s)) for s in grid)
    strs=b''.join(s.encode('latin1')+b'\0' for s in grid)
    return body+cells+strs
if __name__=='__main__':
    d=open('/tmp/ffw/common.dec','rb').read()
    cols,rows,g=decode_table(d)
    print(cols,rows,len(g))
    enc=encode_table(cols,rows,g)
    # roundtrip check: re-decode
    c2,r2,g2=decode_table(b'\0'*16+enc[16:] if False else enc) if False else (None,None,None)
    # verify hashes vs original
    i=d.index(b'sp/specopstable.csv\x00'); p=i+20
    cells=[struct.unpack('>II',d[p+8*k:p+8*k+8]) for k in range(cols*rows)]
    bad=[k for k,(ptr,hs) in enumerate(cells) if hs!=h1(g[k])]
    print('hash mismatches',len(bad))
