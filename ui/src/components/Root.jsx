import React, {useState} from 'react';
import Navigation from './Navigation';
import MenuBar from './MenuBar';
import StatusBar from './StatusBar';
import {Outlet} from 'react-router-dom';
import {HotkeysProvider} from '@blueprintjs/core';
import {WindowManager} from "forge/components";
import { useSetting } from 'forge/core';
import SignIn from './SignIn';

const Root = () => {
    const [isNavigationOpen, setIsNavigationOpen] = useState(true);
    const { useAuth } = useSetting();
    const { profile, ready } = useAuth();

    const toggleNavigation = () => {
        setIsNavigationOpen((prev) => !prev);
    };

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
                        {profile ? (
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
