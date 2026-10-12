from pathlib import Path
import subprocess,os,shutil,hashlib,json,tarfile,io
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory';base=Path('/tmp/toolsy-memory-baseline')
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off')
metadata=json.loads(subprocess.check_output(['go','mod','download','-json','github.com/skosovsky/toolsy@v0.20.1'],cwd=root,env=env,text=True))
assert metadata['Origin']['Hash']=='2b4c9419792bc843ff92c0cf64b35f83330a389d'
shutil.copytree(metadata['Dir'],base,dirs_exist_ok=True)
for p in [base,*base.rglob('*')]:p.chmod(0o755 if p.is_dir() else 0o644)
for name in ['performance_memory_test.go','performance_memory_measure_test.go','performance_boundary_bench_test.go']:shutil.copy2(root/name,base/name)
mcp=json.loads(subprocess.check_output(['go','mod','download','-json','github.com/skosovsky/toolsy/mcp@v0.20.1'],cwd=root,env=env,text=True))
assert mcp['Origin']['Hash']==metadata['Origin']['Hash']
shutil.copytree(mcp['Dir'],base/'mcp',dirs_exist_ok=True)
for p in [base/'mcp',*(base/'mcp').rglob('*')]:p.chmod(0o755 if p.is_dir() else 0o644)
subprocess.run(['go','mod','edit','-replace=github.com/skosovsky/toolsy=../'],cwd=base/'mcp',env=env,check=True)
(out/'baseline-mcp-metadata.json').write_text(json.dumps(mcp,indent=2))
(out/'baseline-metadata.json').write_text(json.dumps(metadata,indent=2))
(out/'fixture-hashes.json').write_text(json.dumps({str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in (root/'testdata/performance').glob('*') if p.is_file()},indent=2))
print(base)
