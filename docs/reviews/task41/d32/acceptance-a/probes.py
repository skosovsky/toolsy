import pathlib,subprocess,json,os
root=pathlib.Path.cwd();out=pathlib.Path('/tmp/toolsy-task41/d32-acceptance-a');env=os.environ.copy();env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache')
checks={
'httptool':'''domains:=[]string{"api.example.com"}; origins:=[]string{"https://api.example.com"}; headers:=map[string]string{"X-Test":"safe"}; opts:=[]Option{WithAllowedDomains(domains),WithCredentialOrigins(origins),WithHeaders(headers)}; mutate:=func(){domains[0]="attacker.invalid"; origins[0]="https://attacker.invalid"; headers["X-Test"]="changed"}; check:=func(){var o options; for _,opt:=range opts{opt(&o)}; if o.allowedDomains[0]!="api.example.com" || o.credentialOrigins[0]!="https://api.example.com" || o.headers["X-Test"]!="safe" {t.Error("snapshot changed")}; o.allowedDomains[0]="own";o.credentialOrigins[0]="own";o.headers["X-Test"]="own"}''',
'web':'''domains:=[]string{"blocked.example"};opt:=WithBlockedDomains(domains);mutate:=func(){domains[0]="changed.invalid"};check:=func(){var o options;opt(&o);if o.blockedDomains[0]!="blocked.example"{t.Error("snapshot changed")};o.blockedDomains[0]="own"}''',
'sqltool':'''tables:=[]string{"public"};opt:=WithAllowedTables(tables);mutate:=func(){tables[0]="private"};check:=func(){var o options;opt(&o);if o.allowedTables[0]!="public"{t.Error("snapshot changed")};o.allowedTables[0]="own"}'''
}
for m,body in checks.items():
 p=out/(m+'-probe_test.go');p.write_text('package '+m+'\nimport("testing";"sync")\nfunc TestAcceptanceAConcurrentOptions(t *testing.T){\n// Arrange: capture policy before mutating its source.\n'+body+'\n// Act: source mutation and independent option materialization overlap.\nvar wg sync.WaitGroup;wg.Add(33);go func(){defer wg.Done();for range 1000{mutate()}}();for range 32{go func(){defer wg.Done();for range 1000{check()}}()};wg.Wait()\n// Assert: checks inside each worker verify policy and independent writable containers.\n}\n')
 overlay=out/(m+'-probe-overlay.json');overlay.write_text(json.dumps({'Replace':{str(root/'toolkits'/m/'option_snapshot_test.go'):str(p)}}))
 with (out/(m+'-probe-race.log')).open('w') as f:r=subprocess.run(['go','test','-overlay='+str(overlay),'-race','-count=3','-run','TestAcceptanceA','./...'],cwd=root/'toolkits'/m,env=env,stdout=f,stderr=subprocess.STDOUT)
 print(m,r.returncode,flush=True)
