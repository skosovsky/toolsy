import pathlib, subprocess, hashlib
root=pathlib.Path.cwd(); rows=[]
mods=['toolkits/fstool']+['adapters/sandbox/'+x for x in ['starlark','host','docker','wazero','e2b']]
for m in mods:
 for p in sorted((root/m).glob('*.go')):
  if p.name.endswith('_test.go') or p.name=='doc.go': continue
  rel=str(p.relative_to(root)); old=subprocess.check_output(['git','show','6907b80:'+rel]); now=p.read_bytes()
  assert old==now, rel
  rows.append(rel+' '+hashlib.sha256(now).hexdigest()+' baseline-identical')
pathlib.Path('/tmp/toolsy-task41/d40-acceptance-b/production-identity.log').write_text('\n'.join(rows)+'\n')
print('baseline-identical production files:',len(rows))
