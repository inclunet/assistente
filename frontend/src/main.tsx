import React from 'react'
import {createRoot} from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'
import { router } from './lib/router'
import './lib/i18n'
import './theme.css'
import './animations.css'
import './style.css'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <RouterProvider router={router} />
    </React.StrictMode>
)
