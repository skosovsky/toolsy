from pathlib import Path
import json,statistics,re
out=Path(__file__).resolve().parent
s=json.loads((out/'bench-summary.json').read_text())
lines=['# Five independent process repeats: final library matrix','', 'Each metric is median [min, max]. Hot setup excluded; cold constructor is separate.', '', '| Benchmark | baseline ns | candidate ns | baseline B/op | candidate B/op | baseline allocs | candidate allocs |','|---|---:|---:|---:|---:|---:|---:|']
for n,v in s.items():
 cols=[]
 for metric in ['ns','bytes','allocs']:
  for r in ['baseline','candidate']:
   d=v[r][metric];cols.append(f"{d['median']:g} [{d['min']:g}, {d['max']:g}]")
 lines.append('| '+n+' | '+' | '.join(cols)+' |')
(out/'bench-table.md').write_text('\n'.join(lines)+'\n')
mem={}
for role in ['baseline','candidate']:
 mem[role]=[]
 for rep in [1,2,3,4]:
  if (out/f'{role}-memory-{rep}/memory.json').exists():mem[role].append(json.loads((out/f'{role}-memory-{rep}/memory.json').read_text()))
(out/'memory-summary.json').write_text(json.dumps(mem,indent=2))
# Boundary repeats are separate independent test processes, untouched implementation.
boundary={}
for role in ['baseline','candidate']:
 values=[]
 for l in (out/f'{role}-boundary.log').read_text().splitlines():
  m=re.match(r'Benchmark\S+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',l)
  if m:values.append(list(map(float,m.groups())))
 boundary[role]={k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])}
(out/'boundary-summary.json').write_text(json.dumps(boundary,indent=2))
consumer=out/'consumer'
if (consumer/'manifest.json').exists() and (consumer/'old-context-r2.json').exists() and (consumer/'old-context-r2.json').stat().st_size:
 result={}
 for kind in ['read','search','context']:
  result[kind]={}
  for role in ['old','official','optimized']:
   records=[json.loads((consumer/f'{role}-{kind}-r{rep}.json').read_text()) for rep in [1,2]]
   fields=['alloc_bytes_per_op','allocations_per_op','p95_seconds','errors','calls','warmup']
   summary={f:[r[f] for r in records] for f in fields}
   summary['mean_bytes']=statistics.mean(summary['alloc_bytes_per_op']);summary['mean_allocs']=statistics.mean(summary['allocations_per_op'])
   summary['retained_range']=[min(e['after_gc']['heap_alloc'] for r in records for e in r['epochs']),max(e['after_gc']['heap_alloc'] for r in records for e in r['epochs'])]
   result[kind][role]=summary
  result[kind]['bytes_target']=result[kind]['old']['mean_bytes']*1.2
  result[kind]['bytes_pass']=result[kind]['optimized']['mean_bytes']<=result[kind]['bytes_target']
 (consumer/'summary.json').write_text(json.dumps(result,indent=2))
