#!/usr/bin/env python3
# Copyright (C) ConfigHub, Inc.
# SPDX-License-Identifier: MIT
"""Serial external map caller; no fleet membership or health inference."""
import argparse
import json
import os
from pathlib import Path
import subprocess


def verified_key(row, cluster):
    evidence = row.get('resourceIdentity', {})
    if not isinstance(evidence, dict):
        return None
    ref = evidence.get('observed', {})
    if not isinstance(ref, dict):
        return None
    if (cluster.get('identity') != 'verified' or cluster.get('omission')
            or not cluster.get('observedAt') or evidence.get('status') != 'verified'):
        return None
    fields = ['clusterIdSource', 'clusterId', 'group', 'kind', 'namespace', 'name', 'uid']
    values = [ref.get(field) for field in fields]
    if not all(isinstance(value, str) for value in values):
        return None
    if not all(ref.get(field) for field in ['clusterIdSource', 'clusterId', 'kind', 'name', 'uid', 'apiVersion']):
        return None
    if ref['clusterId'] != cluster.get('id') or ref['clusterIdSource'] != cluster.get('idSource'):
        return None
    if any(ref[field] != row.get(field, '') for field in ['kind', 'namespace', 'name']):
        return None
    if ref.get('scope') not in ['namespaced', 'cluster']:
        return None
    if (ref['scope'] == 'namespaced') != bool(ref['namespace']):
        return None
    group = ref['apiVersion'].split('/', 1)[0] if '/' in ref['apiVersion'] else ''
    if ref['group'] != group:
        return None
    try:
        decoded = json.loads(evidence.get('mergeKey', ''))
    except (ValueError, TypeError):
        return None
    if decoded != values:
        return None
    return json.dumps(values, ensure_ascii=False, separators=(',', ':'))


def observe(scopes, scout_directory, timeout=30):
    if not isinstance(scopes, list) or not 1 <= len(scopes) <= 32:
        raise ValueError('provide 1 to 32 explicit scopes')
    labels = []
    for scope in scopes:
        if not isinstance(scope, dict) or any(not isinstance(scope.get(k), str) or not scope[k].strip()
                                            for k in ['label', 'context', 'kubeconfig']):
            raise ValueError('each scope requires label, context and kubeconfig strings')
        if not isinstance(scope.get('namespace', ''), str):
            raise ValueError('namespace must be a string')
        labels.append(scope['label'])
    if len(set(labels)) != len(labels):
        raise ValueError('scope labels must be unique; context labels need not be')
    result = {'schema': 'external-map-observations.v1', 'status': 'complete',
              'observations': [], 'instances': []}
    indexed = {}
    for scope in scopes:
        observation = {'label': scope['label'], 'context': scope['context'], 'omissions': []}
        result['observations'].append(observation)
        argv = ['./cub-scout', 'map', 'list', '--cluster-identity', '--format', 'json',
                '--kube-context', scope['context'], '--kind', 'Deployment']
        if scope.get('namespace'):
            argv += ['--namespace', scope['namespace']]
        env = {**os.environ, 'CUB_SCOUT_OFFLINE': 'true', 'KUBECONFIG': str(Path(scope['kubeconfig']).resolve())}
        try:
            process = subprocess.run(argv, cwd=scout_directory, env=env, text=True,
                                     capture_output=True, timeout=timeout)
            if process.returncode:
                raise ValueError('command_failed')
            packet = json.loads(process.stdout)
            if (not isinstance(packet, dict) or packet.get('schema') != 'map-list-cluster-identity.v1'
                    or not isinstance(packet.get('cluster'), dict)
                    or not isinstance(packet.get('resources'), list)
                    or not isinstance(packet.get('collection'), dict)
                    or not all(isinstance(row, dict) for row in packet['resources'])):
                raise ValueError('unsupported_response')
        except subprocess.TimeoutExpired:
            observation['omissions'].append('command_timeout')
        except OSError:
            observation['omissions'].append('command_unavailable')
        except (ValueError, TypeError):
            # Do not echo subprocess stderr, credentials, paths or malformed output.
            observation['omissions'].append('command_failed_or_invalid_response')
        else:
            observation['data'] = packet
            cluster = packet['cluster']
            if cluster.get('identity') != 'verified' or cluster.get('omission'):
                observation['omissions'].append('cluster_identity_unverified')
            if packet['collection'].get('status') != 'complete':
                observation['omissions'].append('collection_incomplete')
            for index, row in enumerate(packet['resources']):
                key = verified_key(row, cluster)
                if key is None:
                    observation['omissions'].append('resource_identity_unverified')
                    continue
                indexed.setdefault(key, []).append({'label': scope['label'], 'resourceIndex': index})
        observation['omissions'] = sorted(set(observation['omissions']))
        if observation['omissions']:
            result['status'] = 'partial'
    result['instances'] = [{'mergeKey': key, 'observations': sorted(members, key=lambda x: (x['label'], x['resourceIndex']))}
                           for key, members in sorted(indexed.items())]
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('scopes', type=Path)
    parser.add_argument('--scout-directory', type=Path, default=Path.cwd())
    args = parser.parse_args()
    print(json.dumps(observe(json.loads(args.scopes.read_text()), args.scout_directory), indent=2))
