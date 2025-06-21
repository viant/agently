import React, {useState} from 'react';
import Navigation from './Navigation';
import MenuBar from './MenuBar';
import StatusBar from './StatusBar';
import {Outlet} from 'react-router-dom';
import {HotkeysProvider} from '@blueprintjs/core';
import {WindowManager} from "forge/components";
import {useSetting} from "forge/core";

const Root = () => {
    const [isNavigationOpen, setIsNavigationOpen] = useState(true);
    const [isAuthenticated, setIsAuthenticated] = useState(true);

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


                    {isAuthenticated && isNavigationOpen && <Navigation/>}
                    <div
                        className="main-content"
                        style={{
                            flex: 1,
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                        }}
                    >
                        {isAuthenticated ? (
                            <>
                                <Outlet/>
                                <WindowManager/>
                            </>
                        ) : (
                          <>no auth</>
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
