import React, {useEffect, useMemo, useRef, useState} from 'react';
import Navigation from './Navigation';
import MenuBar from './MenuBar';
import StatusBar from './StatusBar';
import {Outlet, useLocation} from 'react-router-dom';
import {HotkeysProvider} from '@blueprintjs/core';
import {WindowManager} from "forge/components";
import { addWindow, useSetting } from 'forge/core';
import SignIn from './SignIn';

const parseConversationLink = (loc) => {
    const path = String(loc?.pathname || '');
    const search = String(loc?.search || '');
    const hash = String(loc?.hash || '');

    const fromParams = (params) => {
        if (!params) return { id: '' };
        const convID = params.get('convID') || params.get('conversationId') || params.get('id') || '';
        return { id: String(convID || '').trim() };
    };

    // 1) Standard query string on current path
    const direct = fromParams(new URLSearchParams(search));
    if (direct.id) return direct;

    // 2) Hash-based deep links, e.g. #/chat/new?convID=...
    if (hash) {
        const h = hash.startsWith('#') ? hash.slice(1) : hash;
        const parts = h.split('?');
        const hPath = parts[0] || '';
        if (hPath.startsWith('/chat') || hPath.startsWith('chat')) {
            const params = new URLSearchParams(parts.slice(1).join('?'));
            const parsed = fromParams(params);
            if (parsed.id) return parsed;
        }
        // 3) Hash-only convID
        const params = new URLSearchParams(h.startsWith('?') ? h.slice(1) : '');
        const parsed = fromParams(params);
        if (parsed.id) return parsed;
    }

    // 4) Path-style deep link: /chat/<convID> or /conversation/<convID>
    if (path) {
        const parts = path.split('/').filter(Boolean);
        if ((parts[0] === 'chat' || parts[0] === 'conversation') && parts[1]) {
            return { id: String(parts[1]).trim() };
        }
    }

    return { id: '' };
};

const Root = () => {
    const [isNavigationOpen, setIsNavigationOpen] = useState(true);
    const { useAuth } = useSetting();
    const { profile, ready } = useAuth();
    const location = useLocation();
    const lastDeepLinkRef = useRef('');
    const deepLink = useMemo(() => parseConversationLink(location), [location]);

    const toggleNavigation = () => {
        setIsNavigationOpen((prev) => !prev);
    };

    useEffect(() => {
        if (!ready) return;
        const { id: convID } = deepLink;
        if (!convID) return;

        const key = convID;
        if (lastDeepLinkRef.current === key) return;

        lastDeepLinkRef.current = key;
        const dsParams = {
            conversations: { parameters: { id: convID } },
            messages: { parameters: { convID } },
        };
        addWindow('Chat', null, 'chat/new', null, false, dsParams, { autoIndexTitle: true });
    }, [deepLink, ready, profile]);

    return (
        <>
        <HotkeysProvider>
            <div
                className="root-container"
                style={{display: 'flex', flexDirection: 'column', height: '100vh'}}
            >
                <MenuBar toggleNavigation={toggleNavigation}/>
                <div
                    className="app-container"
                    style={{display: 'flex', flex: 1, overflow: 'hidden'}}
                >
                    {profile && isNavigationOpen && <Navigation/>}
                    <div className="main-content" style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
                        {(profile || deepLink.id) ? (
                          <>
                            <Outlet/>
                            <div style={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
                                <WindowManager/>
                            </div>
                          </>
                        ) : (
                          <SignIn/>
                        )}
                    </div>
                </div>
                <StatusBar/>
            </div>
        </HotkeysProvider>
        </>
    );
};

export default Root;
