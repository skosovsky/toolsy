import pathlib,subprocess,json,os,concurrent.futures,hashlib
root=pathlib.Path.cwd();out=pathlib.Path('/tmp/toolsy-task41/d32-acceptance-a');mods='document fstool httptool human mail prompts rag sqltool timetool web'.split();env=os.environ.copy();env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache')
def job(m):
 dest=out/'baseline'/m;dest.mkdir(parents=True,exist_ok=True);repl={};hashes={}
 files=subprocess.check_output(['git','ls-tree','--name-only','ad4afe2',f'toolkits/{m}/']).decode().splitlines()
 for p in files:
  if not p.endswith('.go') or p.endswith('_test.go'):continue
  data=subprocess.check_output(['git','show','ad4afe2:'+p]);target=dest/(pathlib.Path(p).name+'.txt');target.write_bytes(data);repl[str(root/p)]=str(target);hashes[p]=hashlib.sha256(data).hexdigest()
 overlay=dest/'overlay.json';overlay.write_text(json.dumps({'Replace':repl}));(dest/'sha256.json').write_text(json.dumps(hashes,indent=2))
 with (out/(m+'-baseline-independent.log')).open('w') as f:r=subprocess.run(['go','test','-overlay='+str(overlay),'-run','TestD32','-count=1','./...'],cwd=root/'toolkits'/m,env=env,stdout=f,stderr=subprocess.STDOUT)
 return m,r.returncode,len(repl)
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as ex:
 for r in ex.map(job,mods):print(r,flush=True)
