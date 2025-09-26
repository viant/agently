import React, {useState, useEffect} from 'react';
import {
    Menu,
    MenuItem,
    Popover,
    Button,
    Navbar,
    Alignment,
    InputGroup,
} from '@blueprintjs/core';

import {
  activeWindows,
  selectedTabId,
  bringFloatingWindowToFront,
  useSetting,
} from 'forge/core';





import {useNavigate} from 'react-router-dom';
// ChatWindow component removed – chat implemented via metadata windows.
// History drawer removed – chat features now provided by metadata windows.
import {useSignalEffect} from '@preact/signals-react';
import logo from '../viant-logo.png'; // Import the logo image

const MenuBar = ({toggleNavigation}) => {
    const navigate = useNavigate();
    const [isAuthenticated, setIsAuthenticated] = useState(true);
    const {useAuth} = useSetting();
    const {authStates, defaultAuthProvider} = useAuth();
    const jwtToken = authStates?.[defaultAuthProvider]?.jwtToken;
    // No local chat drawer state.
    // No local history drawer anymore.
    const [searchQuery, setSearchQuery] = useState('');
    const [windowsList, setWindowList] = useState(activeWindows.value || []);

    // Fetch menu definitions after authentication
    useEffect(() => {
        const fetchMenuDefinitions = async () => {
             if (isAuthenticated && jwtToken) {
                // Fetch menu definitions using jwtToken
                log.debug('Fetching menu definitions with token', { token: jwtToken.id_token });
            }
        };
        fetchMenuDefinitions();
    }, []);

    const handleWindowClick = (e, win) => {
        e.preventDefault();
        log.debug('handleWindowClick', win);
        if (win.inTab !== false) {
            // Tabbed window
            selectedTabId.value = win.windowId;
        } else {
            // Floating window
            if (win.isMinimized) {
                // Restore the minimized window
                activeWindows.value = activeWindows.value.map((w) =>
                    w.uniqueWindowKey === win.uniqueWindowKey
                        ? {...w, isMinimized: false}
                        : w
                );
            }
            bringFloatingWindowToFront(win.uniqueWindowKey);
        }
    };

    useSignalEffect(() => {
        setWindowList(activeWindows.value);
    });


    log.debug('windowsList', windowsList)
    const buildWindowsMenu = () => (
        <Menu>
            {windowsList.map((win) => (
                <MenuItem
                    key={"W" + win.uniqueWindowKey}
                    text={win.windowTitle}
                    onClick={(e) => handleWindowClick(e, win)}
                />
            ))}
            {windowsList.length === 0 && <MenuItem text="No windows open" disabled/>}
        </Menu>
    );

    const handleProfileClick = () => {
        navigate('/profile');
    };

    return (
        <Navbar className="menu-bar">
            <Navbar.Group align={Alignment.LEFT}>
                <img src={logo} alt="Logo" style={{height: '30px', marginRight: '10px'}}/>
                <Navbar.Heading>Agently</Navbar.Heading>
                {isAuthenticated && (
                    <>
                        <Navbar.Divider/>
                        <Button
                            icon="menu"
                            minimal
                            title="Toggle Navigation"
                            onClick={toggleNavigation}
                        />
                        <Popover content={buildWindowsMenu()} position="bottom-left">
                            <Button icon="application" minimal title="Open Windows"/>
                        </Popover>
                        {/* Chat handled by navigation (metadata windows) */}
                        <Button icon="notifications" minimal title="Notifications"/>
                    </>
                )}
            </Navbar.Group>
            {isAuthenticated && (
                <Navbar.Group align={Alignment.RIGHT}>
                    <Popover
                        content={
                            <Menu>
                                <MenuItem text="Logout" onClick={() => logout()}/>
                            </Menu>
                        }
                        position="bottom-right"
                    >
                        <Button icon="user" minimal title="User Account"/>
                    </Popover>
                </Navbar.Group>
            )}
        </Navbar>
    );
};

export default MenuBar;
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
