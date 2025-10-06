import React, {useState, useEffect} from 'react';
import {Tree, InputGroup, Spinner} from '@blueprintjs/core';
import {useSetting, addWindow} from 'forge/core';
import {endpoints} from '../endpoint';


const Navigation = () => {
  
    const [hasFetched, setHasFetched] = useState(false);
    const [loading, setLoading] = useState(false);
    const [searchQuery, setSearchQuery] = useState('');
    const [treeData, setTreeData] = useState([]);
    const {connectorConfig = {}, useAuth} = useSetting();
    const {authStates, defaultAuthProvider, profile, ready} = useAuth();
    const jwtToken = authStates?.[defaultAuthProvider]?.jwtToken;

    const config = connectorConfig.navigation
    if (!config) {
        throw new Error("No connectorConfig.navigation found")
    }
    // Helper to build API URL with the configured base
    const apiBase = (endpoints?.appAPI?.baseURL || '').replace(/\/+$/, '');
    const apiURL = (p) => {
        const raw = String(p || '').replace(/^\/+/, '');
        const hasApiInBase = /\/v1\/api\/?$/.test(apiBase);
        const path = hasApiInBase && raw.startsWith('v1/api/') ? raw.replace(/^v1\/api\//, '') : raw;
        return `${apiBase}/${path}`;
    };


    // Fetch navigation data on mount
    useEffect(() => {
        // Fetch once when authenticated (cookie or bearer)
        if (!hasFetched && ready && (profile || jwtToken)) {
            setLoading(true);
            const url = apiURL('/v1/api/agently/forge/navigation');
            const headers = jwtToken ? { Authorization: `Bearer ${jwtToken.id_token || jwtToken}` } : {};
            fetch(url, { credentials: 'include', headers })
                .then(async (resp) => {
                    const json = await resp.json().catch(() => ({}));
                    const items = json?.data || [];
                    const initialTreeData = buildTreeData(items);
                    setTreeData(initialTreeData);
                    setHasFetched(true);
                })
                .catch((e) => { log.error('navigation fetch failed', e?.message || e); })
                .finally(() => setLoading(false));
        }
    }, [ready, profile, jwtToken, hasFetched]);

    if (loading) {
        return <Spinner/>;
    }

    // Handle navigation for leaf nodes
    const handleNavigation = (config) => {
        const {windowKey, windowTitle, windowData} = config;
        log.debug('Adding window', { config, windowKey, windowData });
        let title = windowTitle || windowKey;
        addWindow(title, null, windowKey, windowData);
    };

    // Handle node click
    const handleNodeClick = (nodeData, nodePath) => {
        const wasExpanded = nodeData.isExpanded;
        if (nodeData.childNodes && nodeData.childNodes.length > 0) {
            // If node has children, toggle expansion
            const newTreeData = treeData.map((node, index) => {
                if (index === nodePath[0]) {
                    return updateNodeAtPath(node, nodePath.slice(1), {
                        isExpanded: !wasExpanded,
                    });
                }
                return node;
            });
            setTreeData(newTreeData);
        } else if (nodeData.windowKey) {
            // If node is a leaf with windowKey, open window
            handleNavigation(nodeData);
        } else {
            log.warn('No windowKey defined for node', nodeData);
        }
    };

    // Helper function to update a node's property at a given path
    const updateNodeAtPath = (node, path, updater) => {
        if (path.length === 0) {
            return {
                ...node,
                ...updater,
            };
        }
        const childIndex = path[0];
        const newChildNodes = node.childNodes.map((child, index) => {
            if (index === childIndex) {
                return updateNodeAtPath(child, path.slice(1), updater);
            }
            return child;
        });
        return {
            ...node,
            childNodes: newChildNodes,
        };
    };

    // Build the tree data structure from the navigation data
    const buildTreeData = (nodes) => {


        log.debug('buildTreeData', nodes);
        return nodes.map((node) => ({
            id: node.id,
            label: node.id === 'search' ? (
                <InputGroup
                    leftIcon="search"
                    placeholder="Search..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                />
            ) : (
                node.label
            ),
            icon: node.icon,
            hasCaret: node.childNodes && node.childNodes.length > 0,
            childNodes: node.childNodes ? buildTreeData(node.childNodes) : undefined,
            windowKey: node.windowKey,
            windowTitle: node.windowTitle,
            isExpanded: node.isExpanded || false,
        }));
    };

    return (
        <div
            className="navigation-sidebar"
            style={{
                width: '250px',
                overflowY: 'auto',
                borderRight: '1px solid #ccc',
                backgroundColor: '#f5f8fa',
            }}
        >
            <div className="navigation-tree">
                <Tree contents={treeData} onNodeClick={handleNodeClick}/>
            </div>
        </div>
    );
};

export default Navigation;
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
