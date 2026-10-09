import datetime,hashlib,json,os,pathlib,shutil,subprocess,tempfile,uuid
source=pathlib.Path.cwd();root=pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-watch-context-'));root.chmod(0o700);cfg=root/'admin.json';name='scout-v214-watch-'+uuid.uuid4().hex[:8];created=False;steps=[];failure=None;shared=pathlib.Path.home()/'.kube/config'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None
before=sha(shared);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip();assert not subprocess.check_output(['git','status','--porcelain'],text=True).strip();build=subprocess.run(['go','build','-o',str(root/'cub-scout'),'./cmd/cub-scout'],cwd=source,env={**os.environ,'GOTOOLCHAIN':'go1.26.9'},text=True,capture_output=True,timeout=180);assert build.returncode==0,'isolated build failed';binarysha=sha(root/'cub-scout');started=datetime.datetime.now(datetime.timezone.utc).isoformat();env={**os.environ,'KUBECONFIG':str(cfg),'CUB_SCOUT_OFFLINE':'true','CUB_CONFIG':str(root/'cub-config')};(root/'cub-config').mkdir()
def call(argv,label,input=None,expected=0):
 r=subprocess.run(argv,cwd=root,env=env,input=input,text=True,capture_output=True,timeout=180);(root/(label+'.stdout')).write_text(r.stdout);(root/(label+'.stderr')).write_text(r.stderr);steps.append({'name':label,'exit':r.returncode,'stdoutSHA256':hashlib.sha256(r.stdout.encode()).hexdigest(),'stderrSHA256':hashlib.sha256(r.stderr.encode()).hexdigest()});assert r.returncode==expected,(label,r.returncode,r.stderr[-500:]);return r.stdout
try:
 assert name not in call(['kind','get','clusters'],'before-clusters').splitlines();created=True
 call(['kind','create','cluster','--name',name,'--image','kindest/node:v1.35.0','--kubeconfig',str(cfg),'--wait','90s'],'create-owned-cluster');cfg.chmod(0o600)
 manifest={'apiVersion':'v1','kind':'List','items':[{'apiVersion':'v1','kind':'Namespace','metadata':{'name':'team-a'}},{'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':'api','namespace':'team-a'},'spec':{'replicas':0,'selector':{'matchLabels':{'app':'proof'}},'template':{'metadata':{'labels':{'app':'proof'}},'spec':{'containers':[{'name':'app','image':'registry.k8s.io/pause:3.10'}]}}}}]}
 call(['kubectl','--kubeconfig',str(cfg),'apply','-f','-'],'bootstrap-owned-resources',input=json.dumps(manifest))
 config=json.loads(call(['kubectl','--kubeconfig',str(cfg),'config','view','--raw','-o','json'],'private-config'));config['contexts'][0]['name']='selected';config['current-context']='unusable-ambient';cfg.write_text(json.dumps(config));cfg.chmod(0o600);capturedConfig=sha(cfg)
 resources=[]
 for command in ['watch','bot']:
  output=root/(command+'.jsonl');call(['./cub-scout',command,'--kube-context','selected','--namespace','team-a','--once','--output-file',str(output)],command+'-selected')
  events=[json.loads(x) for x in output.read_text().splitlines()];row=next(x for x in events if x['type']=='resource.discovered' and x['resource']['kind']=='Deployment' and x['resource']['name']=='api');assert row['resource']['namespace']=='team-a';resources.append(row['resource'])
  for selection in ['missing','']:
   bad=root/(command+'-bad-'+('missing' if selection else 'blank')+'.jsonl');call(['./cub-scout',command,'--kube-context',selection,'--namespace','team-a','--once','--output-file',str(bad)],command+'-refuses-'+('missing' if selection else 'blank'),expected=1);assert not bad.exists()
  ambient=root/(command+'-ambient.jsonl');call(['./cub-scout',command,'--namespace','team-a','--once','--output-file',str(ambient)],command+'-ambient-invalid',expected=1);assert not ambient.exists()
 assert resources[0]==resources[1];assert capturedConfig==sha(cfg)
except Exception as e:failure=str(e)
finally:
 cleanup=False
 if created:
  try:call(['kind','delete','cluster','--name',name,'--kubeconfig',str(cfg)],'cleanup-owned-cluster');cleanup=name not in call(['kind','get','clusters'],'after-clusters').splitlines()
  except Exception as e:failure=(failure or '')+' cleanup: '+str(e)
 finalhead=subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip();clean=not subprocess.check_output(['git','status','--porcelain'],cwd=source,text=True).strip()
 proof={'schema':'v214-watch-context-live-proof.v1','sourceCommit':head,'sourceCommitUnchanged':head==finalhead,'sourceWorktreeCleanBeforeAndAfter':clean,'binarySHA256':binarysha,'buildBinding':'The harness built an isolated candidate with Go 1.24 at the clean captured HEAD; binary SHA256 retained. No implicit compiler VCS stamp claim.','started':started,'finished':datetime.datetime.now(datetime.timezone.utc).isoformat(),'passed':failure is None and cleanup and shared.exists() and before==sha(shared) and head==finalhead and clean,'failure':failure,'ownedClusterRemoved':cleanup,'sharedConfigUnchanged':before==sha(shared),'watchBotResourceParity':len(resources)==2 and resources[0]==resources[1],'steps':[x for x in steps if not x['name'].startswith('private-')],'scope':'One owned Kubernetes 1.35 cluster; actual watch and bot --once select a valid private context despite unusable ambient context. Missing/blank contexts and invalid ambient selection refuse before file creation. No verified cluster identity, complete scanner coverage, informer age/reconnect, total cost, Target or health claim.'}
 (root/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'proof':str(root/'proof.json'),'passed':proof['passed'],'failure':failure}));assert proof['passed']
