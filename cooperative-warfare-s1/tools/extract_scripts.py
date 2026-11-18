# MW3 co-op patch v0.0.50 - S1 script extractor (reads a big .ff, writes a SMALL zip of just the scripts)
import sys,zlib,struct,re,zipfile,os
src=sys.argv[1]; out=sys.argv[2] if len(sys.argv)>2 else os.path.splitext(os.path.basename(src))[0]+'_scripts.zip'
print('MW3 co-op patch v0.0.50 - extracting scripts from',src)
PAT=re.compile(rb'((?:maps|common_scripts|vehicle_scripts|animscripts|codescripts)/[A-Za-z0-9_/\.]+?)\x00')
CH=64<<20; OV=12<<20
f=open(src,'rb')
head=f.read(128); print('header:',head[:48].hex())
def ok_hdr(b):
    return len(b)>=2 and b[0]==0x78 and b[1] in (0x01,0x5e,0x9c,0xda)
start=None
for off in (40,36,44,48,32,52,56,64,72,80,96,128):
    if ok_hdr(head[off:off+2]):
        try:
            f.seek(off); t=zlib.decompressobj().decompress(f.read(65536)); start=off; break
        except Exception: pass
if start is None:
    print('no zlib start found at the usual offsets; searching...'); f.seek(0); blk=f.read(1<<20)
    for i in range(len(blk)-2):
        if ok_hdr(blk[i:i+2]):
            try: zlib.decompressobj().decompress(blk[i:i+4096]); start=i; break
            except Exception: pass
print('zlib stream starts at',start)
if start is None: sys.exit('could not find a zlib stream - send me the header line printed above')
f.seek(start); d=zlib.decompressobj()
buf=b''; base=0; found={}; done=False
SPAT=re.compile(rb'[A-Z][A-Z0-9_]{5,}\x00')
strs=[]
def scan(b,base,final):
    lim=len(b) if final else len(b)-OV
    for m in SPAT.finditer(b):
        if m.start()>=lim: break
        k=m.group(0)[:-1]
        if (b'FUSION' in k or b'LOAD' in k or b'LEVEL' in k or b'MISSION' in k or b'INTRO' in k) and len(strs)<20000:
            c=b[max(0,m.start()-160):m.end()+160]
            strs.append((base+m.start(),k.decode(),c))
    for m in PAT.finditer(b):
        s=m.start()
        if s>=lim: break
        name=m.group(1).decode()
        e=m.end()
        if s>=24:
            w=struct.unpack('>6I',b[s-24:s])
            if w[0]==0xffffffff and w[4]==0xffffffff and w[5]==0xffffffff and 0<w[1]<1<<24 and 0<w[2]<1<<26 and w[3]<1<<26 and not name.endswith('.gsc'):
                try:
                    stack=zlib.decompress(b[e:e+w[1]]); code=b[e+w[1]:e+w[1]+w[3]]
                    if len(stack)==w[2] and len(code)==w[3]: found.setdefault(('c',name),(stack,code,base+s))
                    continue
                except Exception: pass
        if s>=16:
            w=struct.unpack('>4I',b[s-16:s])
            if w[0]==0xffffffff and w[3]==0xffffffff and 0<w[1]<1<<24 and 0<w[2]<1<<26 and name.endswith('.gsc'):
                try:
                    t=zlib.decompress(b[e:e+w[1]])
                    if len(t)==w[2]: found.setdefault(('r',name),(t,None,base+s))
                except Exception: pass
n=0
while True:
    raw=f.read(8<<20)
    if not raw: break
    pend=raw
    while pend:
        try: buf+=d.decompress(pend)
        except zlib.error:
            break
        if d.eof:
            pend=d.unused_data; d=zlib.decompressobj()
            k=0
            while k<min(len(pend),16) and not (ok_hdr(pend[k:k+2])): k+=1
            pend=pend[k:]
        else: pend=b''
    n+=len(raw)
    if len(buf)>CH+OV:
        scan(buf,base,False)
        keep=buf[-OV:]; base+=len(buf)-OV; buf=keep
    print('\r%d MB read, %d scripts' % (n>>20,len(found)),end='',flush=True)
buf+=d.flush(); scan(buf,base,True)
print('\ndone:',len(found),'scripts')
with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED) as z:
    for (k,name),(a,b,off) in found.items():
        if k=='c': z.writestr(name+'.cgsc.stack',a); z.writestr(name+'.cgsc',b)
        else: z.writestr(name,a)
with zipfile.ZipFile(out,'a',zipfile.ZIP_DEFLATED) as z:
    seen=set(); lines=[]
    for off,k,c in strs:
        if k in seen: continue
        seen.add(k); lines.append(k+'\t'+''.join(chr(x) if 32<=x<127 else '.' for x in c))
    z.writestr('_strings_context.txt','\n'.join(lines))
print('wrote',out,os.path.getsize(out),'bytes')
