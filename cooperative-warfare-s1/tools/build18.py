import zlib,struct,glob,sys,os
def build(assets,out):
    n=len(assets)
    lst=struct.pack('>IIII',0,0,n,0xffffffff)+struct.pack('>I',0)+b''.join(struct.pack('>II',t,0xffffffff) for t,_ in assets)
    body=b''.join(x for _,x in assets)
    blocks=[0x1000]+[0x40000]*6
    x=struct.pack('>II',0,0)+b''.join(struct.pack('>I',v) for v in blocks)+lst+body
    x=struct.pack('>I',len(x)-36)+x[4:]
    z=zlib.compress(x,1);total=40+len(z)
    head=b'IWffu100'+struct.pack('>IIII',0x4f2,0x404,0x01ce4854,0xd9e50240)+struct.pack('>IIII',1,0,total,total)
    open(out,'wb').write(head+z)
f=glob.glob('/tmp/h18/v17/**/patch.ff',recursive=True)[0]
x=zlib.decompress(open(f,'rb').read()[40:])
menu,table,l1,l2=x[88:117],x[117:13460],x[13460:13504],x[13504:]
def script(name,stack,code):
    zs=zlib.compress(stack,9)
    return struct.pack('>IIIIII',0xffffffff,len(zs),len(stack),len(code),0xffffffff,0xffffffff)+name.encode()+b'\0'+zs+code
c='/tmp/h18/compiled/iw5/'
sc=script('maps/_player_death',open(c+'_player_death.cgsc.stack','rb').read(),open(c+'_player_death.cgsc','rb').read())
build([(26,menu),(42,table),(29,l1),(29,l2),(41,sc)],sys.argv[1])
