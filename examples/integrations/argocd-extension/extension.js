/**
 * Argo CD CCVE Extension
 *
 * Displays Config CVE findings for applications in the Argo CD UI.
 *
 * Extension types:
 * 1. Status Panel - Shows CCVE count badge
 * 2. Application Tab - Detailed findings list
 * 3. Resource Tab - Per-resource CCVE info
 */

((window) => {
  const React = window.React;
  const { useState, useEffect } = React;

  // CCVE severity colors
  const SEVERITY_COLORS = {
    critical: '#dc3545',
    warning: '#ffc107',
    info: '#17a2b8'
  };

  // Fetch CCVEs for an application
  async function fetchCCVEs(appName, namespace) {
    try {
      // Option 1: From ConfigMap created by scanner
      const response = await fetch(`/api/v1/applications/${appName}/resource?namespace=${namespace}&resourceName=ccve-findings&kind=ConfigMap&group=&version=v1`);
      if (response.ok) {
        const data = await response.json();
        return JSON.parse(data.manifest).data?.findings || '[]';
      }

      // Option 2: From backend service (if configured)
      const backendResponse = await fetch(`/extensions/ccve/api/findings?app=${appName}`);
      if (backendResponse.ok) {
        return await backendResponse.json();
      }

      return [];
    } catch (e) {
      console.error('Failed to fetch CCVEs:', e);
      return [];
    }
  }

  // Status Panel Extension - Shows badge in status bar
  const CCVEStatusPanel = ({ application }) => {
    const [findings, setFindings] = useState([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
      fetchCCVEs(application.metadata.name, application.metadata.namespace)
        .then(f => {
          setFindings(JSON.parse(f) || []);
          setLoading(false);
        });
    }, [application]);

    if (loading) return null;
    if (findings.length === 0) return null;

    const critical = findings.filter(f => f.severity === 'critical').length;
    const warning = findings.filter(f => f.severity === 'warning').length;
    const info = findings.filter(f => f.severity === 'info').length;

    const color = critical > 0 ? SEVERITY_COLORS.critical :
                  warning > 0 ? SEVERITY_COLORS.warning :
                  SEVERITY_COLORS.info;

    return React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        padding: '4px 8px',
        borderRadius: '4px',
        backgroundColor: color,
        color: 'white',
        fontSize: '12px',
        fontWeight: 'bold'
      }
    }, [
      React.createElement('span', { key: 'icon' }, '⚠️ '),
      React.createElement('span', { key: 'count' },
        `${findings.length} CCVE${findings.length > 1 ? 's' : ''}`
      )
    ]);
  };

  // Flyout panel with details
  const CCVEFlyout = ({ application }) => {
    const [findings, setFindings] = useState([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
      fetchCCVEs(application.metadata.name, application.metadata.namespace)
        .then(f => {
          setFindings(JSON.parse(f) || []);
          setLoading(false);
        });
    }, [application]);

    if (loading) {
      return React.createElement('div', {}, 'Loading CCVEs...');
    }

    if (findings.length === 0) {
      return React.createElement('div', {
        style: { padding: '20px', textAlign: 'center', color: '#28a745' }
      }, '✓ No Config CVEs detected');
    }

    return React.createElement('div', { style: { padding: '10px' } }, [
      React.createElement('h3', { key: 'title' }, 'Config CVE Findings'),
      React.createElement('table', {
        key: 'table',
        style: { width: '100%', borderCollapse: 'collapse' }
      }, [
        React.createElement('thead', { key: 'thead' },
          React.createElement('tr', {}, [
            React.createElement('th', { key: 'id', style: { textAlign: 'left', padding: '8px' } }, 'ID'),
            React.createElement('th', { key: 'sev', style: { textAlign: 'left', padding: '8px' } }, 'Severity'),
            React.createElement('th', { key: 'res', style: { textAlign: 'left', padding: '8px' } }, 'Resource'),
          ])
        ),
        React.createElement('tbody', { key: 'tbody' },
          findings.map((f, i) =>
            React.createElement('tr', { key: i, style: { borderTop: '1px solid #ddd' } }, [
              React.createElement('td', { key: 'id', style: { padding: '8px' } },
                React.createElement('a', {
                  href: `https://ccve.dev/${f.id}`,
                  target: '_blank',
                  style: { color: '#0066cc' }
                }, f.id)
              ),
              React.createElement('td', { key: 'sev', style: { padding: '8px' } },
                React.createElement('span', {
                  style: {
                    padding: '2px 6px',
                    borderRadius: '3px',
                    backgroundColor: SEVERITY_COLORS[f.severity] || '#666',
                    color: 'white',
                    fontSize: '11px'
                  }
                }, f.severity.toUpperCase())
              ),
              React.createElement('td', { key: 'res', style: { padding: '8px', fontFamily: 'monospace' } }, f.resource)
            ])
          )
        )
      ])
    ]);
  };

  // Application Tab Extension - Full CCVE view
  const CCVEApplicationTab = ({ application, tree }) => {
    const [findings, setFindings] = useState([]);
    const [loading, setLoading] = useState(true);
    const [selectedCCVE, setSelectedCCVE] = useState(null);

    useEffect(() => {
      fetchCCVEs(application.metadata.name, application.metadata.namespace)
        .then(f => {
          setFindings(JSON.parse(f) || []);
          setLoading(false);
        });
    }, [application]);

    if (loading) {
      return React.createElement('div', {
        style: { padding: '40px', textAlign: 'center' }
      }, 'Scanning for Config CVEs...');
    }

    // Group by severity
    const grouped = {
      critical: findings.filter(f => f.severity === 'critical'),
      warning: findings.filter(f => f.severity === 'warning'),
      info: findings.filter(f => f.severity === 'info')
    };

    return React.createElement('div', { style: { padding: '20px' } }, [
      // Summary cards
      React.createElement('div', {
        key: 'summary',
        style: { display: 'flex', gap: '20px', marginBottom: '20px' }
      }, [
        createSummaryCard('Critical', grouped.critical.length, SEVERITY_COLORS.critical),
        createSummaryCard('Warning', grouped.warning.length, SEVERITY_COLORS.warning),
        createSummaryCard('Info', grouped.info.length, SEVERITY_COLORS.info)
      ]),

      // Findings list
      findings.length === 0
        ? React.createElement('div', {
            key: 'empty',
            style: { padding: '40px', textAlign: 'center', color: '#28a745', fontSize: '18px' }
          }, '✓ No Config CVEs detected')
        : React.createElement('div', { key: 'list' },
            findings.map((f, i) => createFindingCard(f, i, selectedCCVE, setSelectedCCVE))
          )
    ]);
  };

  function createSummaryCard(label, count, color) {
    return React.createElement('div', {
      key: label,
      style: {
        padding: '15px 25px',
        backgroundColor: count > 0 ? color : '#f5f5f5',
        color: count > 0 ? 'white' : '#666',
        borderRadius: '8px',
        textAlign: 'center',
        minWidth: '100px'
      }
    }, [
      React.createElement('div', {
        key: 'count',
        style: { fontSize: '28px', fontWeight: 'bold' }
      }, count),
      React.createElement('div', {
        key: 'label',
        style: { fontSize: '12px', textTransform: 'uppercase' }
      }, label)
    ]);
  }

  function createFindingCard(finding, index, selectedCCVE, setSelectedCCVE) {
    const isExpanded = selectedCCVE === finding.id;

    return React.createElement('div', {
      key: index,
      style: {
        border: '1px solid #ddd',
        borderLeft: `4px solid ${SEVERITY_COLORS[finding.severity]}`,
        borderRadius: '4px',
        marginBottom: '10px',
        backgroundColor: 'white'
      }
    }, [
      // Header (clickable)
      React.createElement('div', {
        key: 'header',
        onClick: () => setSelectedCCVE(isExpanded ? null : finding.id),
        style: {
          padding: '12px 15px',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }
      }, [
        React.createElement('div', { key: 'left' }, [
          React.createElement('span', {
            key: 'id',
            style: { fontWeight: 'bold', marginRight: '10px' }
          }, finding.id),
          React.createElement('span', {
            key: 'sev',
            style: {
              padding: '2px 6px',
              borderRadius: '3px',
              backgroundColor: SEVERITY_COLORS[finding.severity],
              color: 'white',
              fontSize: '10px',
              marginRight: '10px'
            }
          }, finding.severity.toUpperCase()),
          React.createElement('span', {
            key: 'res',
            style: { fontFamily: 'monospace', color: '#666' }
          }, finding.resource)
        ]),
        React.createElement('span', { key: 'arrow' }, isExpanded ? '▼' : '▶')
      ]),

      // Expandable details
      isExpanded && React.createElement('div', {
        key: 'details',
        style: {
          padding: '15px',
          borderTop: '1px solid #eee',
          backgroundColor: '#fafafa'
        }
      }, [
        finding.message && React.createElement('p', { key: 'msg' }, finding.message),
        finding.remediation && React.createElement('div', { key: 'fix' }, [
          React.createElement('strong', { key: 'label' }, 'Remediation: '),
          React.createElement('p', { key: 'text' }, finding.remediation)
        ]),
        React.createElement('a', {
          key: 'link',
          href: `https://ccve.dev/${finding.id}`,
          target: '_blank',
          style: { color: '#0066cc' }
        }, 'View full documentation →')
      ])
    ]);
  }

  // Resource Tab Extension - Show CCVEs for specific resource
  const CCVEResourceTab = ({ resource, application }) => {
    const [findings, setFindings] = useState([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
      fetchCCVEs(application.metadata.name, application.metadata.namespace)
        .then(f => {
          const allFindings = JSON.parse(f) || [];
          // Filter to this resource
          const resourceKey = `${resource.namespace}/${resource.kind}/${resource.name}`;
          setFindings(allFindings.filter(f => f.resource === resourceKey));
          setLoading(false);
        });
    }, [application, resource]);

    if (loading) return React.createElement('div', {}, 'Loading...');
    if (findings.length === 0) {
      return React.createElement('div', { style: { padding: '20px', color: '#28a745' } },
        '✓ No CCVEs for this resource'
      );
    }

    return React.createElement('div', { style: { padding: '10px' } },
      findings.map((f, i) => createFindingCard(f, i, null, () => {}))
    );
  };

  // Register extensions
  const extensionsAPI = window.extensionsAPI;

  // 1. Status Panel Extension
  extensionsAPI.registerStatusPanelExtension({
    title: 'CCVEs',
    flyout: CCVEFlyout,
    icon: () => React.createElement('span', {}, '⚠️'),
    component: CCVEStatusPanel
  });

  // 2. Application Tab Extension
  extensionsAPI.registerApplicationTabExtension({
    title: 'CCVEs',
    icon: () => React.createElement('span', {}, '🔍'),
    component: CCVEApplicationTab
  });

  // 3. Resource Tab Extension
  extensionsAPI.registerResourceExtension({
    group: '',
    kind: 'Deployment',
    tabTitle: 'CCVEs',
    component: CCVEResourceTab
  });

  extensionsAPI.registerResourceExtension({
    group: '',
    kind: 'StatefulSet',
    tabTitle: 'CCVEs',
    component: CCVEResourceTab
  });

  console.log('CCVE Extension loaded');

})(window);
