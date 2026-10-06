import pathlib,subprocess,os,json
root=pathlib.Path.cwd();out=pathlib.Path('/tmp/toolsy-task41/d32-acceptance-a');env=os.environ.copy();env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache')
p=out/'fstool-probe_test.go';p.write_text('''package fstool
import("testing";"errors";"os")
func TestAcceptanceAInvalidBeforeRootIO(t *testing.T){
 // Arrange.
 missing:=t.TempDir()+"/missing"
 for _,opt:=range []Option{nil,WithMaxBytes(-1),WithMaxEntries(-1)}{
 // Act.
 tools,err:=AsTools(missing,opt)
 // Assert.
 if err==nil || tools!=nil || errors.Is(err,os.ErrNotExist){t.Fatalf("configuration not checked before root IO: %v",err)}
 }
}
''');overlay=out/'fstool-probe-overlay.json';overlay.write_text(json.dumps({'Replace':{str(root/'toolkits/fstool/constructor_contract_test.go'):str(p)}}))
for name,args in [('fstool-probe-race',['go','test','-overlay='+str(overlay),'-race','-count=3','-run','TestAcceptanceA','./...']),('fstool-final-race',['go','test','-race','-count=3','./...']),('fstool-final-lint',['/opt/homebrew/bin/golangci-lint','run','--allow-parallel-runners','./...'])]:
 env['GOLANGCI_LINT_CACHE']=str(out/'fstool-final-lintcache')
 with (out/(name+'.log')).open('w') as f:r=subprocess.run(args,cwd=root/'toolkits/fstool',env=env,stdout=f,stderr=subprocess.STDOUT)
 print(name,r.returncode,flush=True)
m='memory';dest=out/'baseline'/m;dest.mkdir(parents=True,exist_ok=True);repl={}
files=subprocess.check_output(['git','ls-tree','--name-only','ad4afe2',f'toolkits/{m}/']).decode().splitlines()
for rel in files:
 if not rel.endswith('.go'):continue
 target=dest/(pathlib.Path(rel).name+'.txt');target.write_bytes(subprocess.check_output(['git','show','ad4afe2:'+rel]));repl[str(root/rel)]=str(target)
blank=dest/'blank.txt';blank.write_text('package memory\n')
for current in (root/'toolkits/memory').glob('*_test.go'):
 if str(current) not in repl:repl[str(current)]=str(blank)
p=dest/'probe.txt';p.write_text('''package memory
import "testing"
func TestAcceptanceABaselineNil(t *testing.T){
 // Arrange/Act: old-signature constructor invocation.
 defer func(){if r:=recover();r!=nil {t.Errorf("nil option panics: %v",r)}}()
 NewScratchpad(nil)
}
func TestAcceptanceABaselineNegative(t *testing.T){
 // Arrange/Act.
 pad:=NewScratchpad(WithMaxFacts(-1))
 // Assert: baseline exposes an invalid usable instance before AsTools.
 if pad!=nil {t.Error("negative constructor configuration returns usable object")}
}
''');repl[str(root/'toolkits/memory/constructor_contract_test.go')]=str(p);overlay=dest/'overlay.json';overlay.write_text(json.dumps({'Replace':repl}))
with (out/'memory-baseline-independent.log').open('w') as f:r=subprocess.run(['go','test','-overlay='+str(overlay),'-run','TestAcceptanceA','-count=1','./...'],cwd=root/'toolkits/memory',env=env,stdout=f,stderr=subprocess.STDOUT)
print('memory baseline',r.returncode)
