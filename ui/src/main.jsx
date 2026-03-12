import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import './index.css'
import App from './App.jsx'
import {installComposerHistoryEnhancer} from './utils/composerHistoryEnhancer.js'

installComposerHistoryEnhancer()

createRoot(document.getElementById('root')).render(
    <App/>
)
