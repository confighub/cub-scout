import datetime,fcntl,hashlib,http.server,json,os,pathlib,pty,select,shutil,struct,subprocess,tempfile,time,uuid
root=pathlib.Path(tempfile.mkdtemp(prefix='scout-v214-instance-live-'));os.chmod(root,0o700);created=[];steps=[];failure=None;cleaned=[]
source=pathlib.Path.cwd();shared=pathlib.Path.home()/'.kube/config'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None
before=sha(shared);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip();assert not subprocess.check_output(['git','status','--porcelain'],text=True).strip();started=datetime.datetime.now(datetime.timezone.utc).isoformat()
envbase=os.environ.copy();envbase.update({'CUB_SCOUT_OFFLINE':'true','CUB_CONFIG':str(root/'cub-config'),'GOTOOLCHAIN':'go1.26.9'});(root/'cub-config').mkdir()
def call(argv,label,env=None,input=None,expected=0,cwd=None):
 r=subprocess.run(argv,input=input,env=env or envbase,cwd=cwd or source,text=True,capture_output=True,timeout=180);(root/(label+'.stdout')).write_text(r.stdout);(root/(label+'.stderr')).write_text(r.stderr);steps.append({'name':label,'exit':r.returncode,'stdoutSHA256':hashlib.sha256(r.stdout.encode()).hexdigest(),'stderrSHA256':hashlib.sha256(r.stderr.encode()).hexdigest()});assert r.returncode==expected,(label,r.returncode,r.stderr[-600:]);return r.stdout
configs=[];reports=[];keys=[];uids=[];uiPassed=False;mcpPassed=False;restrictedPassed=False
try:
 call(['go','build','-buildvcs=true','-o',str(root/'cub-scout'),'./cmd/cub-scout'],'build-clean-source')
 for index in range(2):
  name='scout-v214-inst-'+uuid.uuid4().hex[:8];cfg=root/f'cluster-{index}.yaml';assert name not in call(['kind','get','clusters'],f'preflight-{index}').splitlines();created.append((name,cfg));env=envbase.copy();env['KUBECONFIG']=str(cfg)
  call(['kind','create','cluster','--name',name,'--image','kindest/node:v1.35.0','--kubeconfig',str(cfg),'--wait','90s'],f'create-{index}',env=env);os.chmod(cfg,0o600)
  call(['kubectl','--kubeconfig',str(cfg),'config','rename-context','kind-'+name,'same-label'],f'private-context-{index}',env=env)
  manifest={'apiVersion':'v1','kind':'List','items':[{'apiVersion':'v1','kind':'Namespace','metadata':{'name':'team-a'}},{'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':'api','namespace':'team-a'},'spec':{'replicas':0,'selector':{'matchLabels':{'app':'proof'}},'template':{'metadata':{'labels':{'app':'proof'}},'spec':{'containers':[{'name':'app','image':'registry.k8s.io/pause:3.10'}]}}}}]}
  call(['kubectl','--kubeconfig',str(cfg),'apply','-f','-'],f'bootstrap-{index}',env=env,input=json.dumps(manifest))
  clusterUid=json.loads(call(['kubectl','--kubeconfig',str(cfg),'get','namespace','kube-system','-o','json'],f'cluster-uid-{index}',env=env))['metadata']['uid']
  objectUid=json.loads(call(['kubectl','--kubeconfig',str(cfg),'get','deployment','api','-n','team-a','-o','json'],f'object-uid-{index}',env=env))['metadata']['uid'];uids.append(objectUid)
  common=['./cub-scout','map','list','--kube-context','same-label','--namespace','team-a','--kind','Deployment','--cluster-identity']
  report=json.loads(call(common+['--format','json'],f'map-{index}',env=env,cwd=root));row=next(x for x in report['resources'] if x['name']=='api');ref=row['resourceIdentity'];assert ref['status']=='verified';assert ref['observed']['uid']==objectUid;assert ref['observed']['clusterId']==clusterUid;assert report['cluster']['id']==clusterUid;assert report['cluster']['cost']['requestsMade']==1;keys.append(ref['mergeKey']);reports.append(report);configs.append(cfg)
  for fmt in ('ascii','md'):assert objectUid in call(common+['--format',fmt],f'map-{index}-{fmt}',env=env,cwd=root)
 assert keys[0]!=keys[1];assert reports[0]['cluster']['context']==reports[1]['cluster']['context']=='same-label'
 cfg=configs[0];env=envbase.copy();env['KUBECONFIG']=str(cfg)
 requests=[{'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2024-11-05','capabilities':{},'clientInfo':{'name':'local-owned-proof','version':'1'}}},{'jsonrpc':'2.0','method':'notifications/initialized'},{'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'map','arguments':{'context':'same-label','namespace':'team-a','cluster_identity':True}}}]
 out=call(['./cub-scout','mcp','serve'],'actual-mcp',env=env,cwd=root,input=''.join(json.dumps(x)+'\n' for x in requests));responses={x['id']:x for x in map(json.loads,out.splitlines()) if 'id' in x};result=responses[2]['result'];assert not result.get('isError');data=result['structuredContent']['data'];mcpRow=next(x for x in data['resources'] if x['name']=='api');assert mcpRow['resourceIdentity']==reports[0]['resources'][0]['resourceIdentity'];mcpPassed=True
 # Actual live standalone TUI, one owned PTY with its candidate child only.
 master,slave=pty.openpty();fcntl.ioctl(slave,0x80087467,struct.pack('HHHH',80,400,0,0));child=subprocess.Popen(['./cub-scout','map','--cluster-identity','--kube-context','same-label','--namespace','team-a'],cwd=root,env={**env,'TERM':'xterm-256color'},stdin=slave,stdout=slave,stderr=slave,start_new_session=True);os.close(slave);terminal=bytearray();deadline=time.monotonic()+25;lastV=0
 try:
  while time.monotonic()<deadline and child.poll() is None:
   if time.monotonic()-lastV>1:os.write(master,b'V');lastV=time.monotonic()
   ready,_,_=select.select([master],[],[],0.3)
   if ready:
    try: terminal.extend(os.read(master,65536))
    except OSError:break
   if uids[0].encode() in terminal and b'Object identity:' in terminal:uiPassed=True;break
 finally:
  if child.poll() is None:os.write(master,b'\x03')
  try:child.wait(timeout=5)
  except subprocess.TimeoutExpired:
   child.terminate()
   try:child.wait(timeout=5)
   except subprocess.TimeoutExpired:child.kill();child.wait(timeout=5)
  os.close(master);(root/'actual-tui.terminal').write_bytes(terminal)
 assert uiPassed,'actual TUI UID evidence not visible before deadline'
 call(['kubectl','--kubeconfig',str(cfg),'delete','deployment','api','-n','team-a'], 'delete-owned-object',env=env)
 recreated=manifest['items'][1];call(['kubectl','--kubeconfig',str(cfg),'apply','-f','-'],'recreate-owned-object',env=env,input=json.dumps(recreated))
 after=json.loads(call(['./cub-scout','map','list','--kube-context','same-label','--namespace','team-a','--kind','Deployment','--cluster-identity','--format','json'],'map-recreated',env=env,cwd=root));newRow=after['resources'][0];assert newRow['name']=='api';assert newRow['resourceIdentity']['observed']['uid']!=uids[0];assert newRow['resourceIdentity']['mergeKey']!=keys[0]
 # Restricted viewer retains the workload while refusing cluster/object merge keys.
 role={'apiVersion':'v1','kind':'List','items':[{'apiVersion':'v1','kind':'ServiceAccount','metadata':{'name':'viewer','namespace':'team-a'}},{'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole','metadata':{'name':'instance-proof-reader'},'rules':[{'apiGroups':['','apps','batch'],'resources':['pods','deployments','statefulsets','daemonsets','jobs','cronjobs','services','configmaps'],'verbs':['get','list']}]},{'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding','metadata':{'name':'instance-proof-reader'},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'instance-proof-reader'},'subjects':[{'kind':'ServiceAccount','name':'viewer','namespace':'team-a'}]}]}
 call(['kubectl','--kubeconfig',str(cfg),'apply','-f','-'],'bootstrap-reader',env=env,input=json.dumps(role))
 token=call(['kubectl','--kubeconfig',str(cfg),'-n','team-a','create','token','viewer','--duration','10m'],'private-reader-token',env=env).strip()
 config=json.loads(call(['kubectl','--kubeconfig',str(cfg),'config','view','--raw','-o','json'],'private-reader-config',env=env));config['users']=[{'name':'viewer','user':{'token':token}}];config['contexts']=[{'name':'same-label','context':{'cluster':config['clusters'][0]['name'],'user':'viewer','namespace':'team-a'}}];config['current-context']='same-label';viewer=root/'reader.yaml';viewer.write_text(json.dumps(config));os.chmod(viewer,0o600);readerEnv={**env,'KUBECONFIG':str(viewer)}
 denied=json.loads(call(['./cub-scout','map','list','--kube-context','same-label','--namespace','team-a','--kind','Deployment','--cluster-identity','--format','json'],'restricted-map',env=readerEnv,cwd=root));row=next(x for x in denied['resources'] if x['name']=='api');assert denied['cluster']['identity']=='unverified';assert row['resourceIdentity']['status']=='unverified';assert 'mergeKey' not in row['resourceIdentity'];assert row['resourceIdentity']['omission']=='cluster_identity_unverified'
 call(['kubectl','--kubeconfig',str(viewer),'get','namespace','kube-system'],'restricted-actual-namespace-denied',expected=1,env=readerEnv)
 restrictedPassed=True
except Exception as e:failure=str(e)
finally:
 for name,cfg in reversed(created):
  try:call(['kind','delete','cluster','--name',name,'--kubeconfig',str(cfg)],'cleanup-'+name);cleaned.append(name)
  except Exception as e:failure=(failure or '')+' cleanup: '+str(e)
 remaining=call(['kind','get','clusters'],'after-clusters').splitlines();cleanup=len(cleaned)==len(created) and all(name not in remaining for name,_ in created);finishhead=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip();clean=not subprocess.check_output(['git','status','--porcelain'],text=True).strip()
 proof={'schema':'v214-map-instance-live-proof.v1','sourceCommit':head,'sourceCommitUnchanged':head==finishhead,'sourceWorktreeCleanBeforeAndAfter':clean,'binarySHA256':sha(root/'cub-scout'),'buildCommand':['go','build','-buildvcs=true','-o','<private>/cub-scout','./cmd/cub-scout'],'started':started,'finished':datetime.datetime.now(datetime.timezone.utc).isoformat(),'passed':failure is None and cleanup and before==sha(shared) and head==finishhead and clean,'failure':failure,'ownedClustersRemoved':cleanup,'sharedConfigUnchanged':before==sha(shared),'sharedConfigSHA256':before,'distinctClusterKeys':len(keys)==2 and keys[0]!=keys[1],'actualMCPMatchedCLI':mcpPassed,'actualTUIVisibleUID':uiPassed,'restrictedReaderRetainedInventoryWithoutKey':restrictedPassed,'steps':[s for s in steps if not s['name'].startswith('private-')],'scope':'Two disposable Kubernetes 1.35 clusters with matching context/object names; actual CLI ascii/json/md, MCP stdio, standalone TUI UID and object recreation. Not broader controllers, connected Target, read-cost coverage, freshness, application health or full six-surface conformance.'}
 (root/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'proof':str(root/'proof.json'),'passed':proof['passed'],'failure':failure}));assert proof['passed']
