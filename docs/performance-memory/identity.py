from pathlib import Path
import subprocess,hashlib,json
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
files={}
for p in root.rglob('*'):
 rel=p.relative_to(root)
 if any(x.startswith('.') for x in rel.parts) or rel.parts[0]=='docs' or not p.is_file():continue
 if p.suffix in ['.go','.json','.sum','.mod','.sh','.yaml','.yml'] or p.name in ['Makefile','LICENSE']:
  files[str(rel)]=hashlib.sha256(p.read_bytes()).hexdigest()
files['.golangci.yml']=hashlib.sha256((root/'.golangci.yml').read_bytes()).hexdigest()
encoded=json.dumps(files,sort_keys=True,separators=(',',':')).encode()
record={'head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip(),'inventory_sha256':hashlib.sha256(encoded).hexdigest(),'files':files,'tracked_diff_sha256':hashlib.sha256(subprocess.check_output(['git','diff','--binary','--','*.go'],cwd=root)).hexdigest()}
(out/'candidate-identity.json').write_text(json.dumps(record,indent=2))
print(record['inventory_sha256'])
