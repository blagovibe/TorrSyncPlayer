import React, { useState } from 'react'
import { HomePage } from './components/HomePage'
import { RoomPage } from './components/RoomPage'
import ErrorBoundary from './components/ErrorBoundary'
import './App.css'

type AppView = 'home' | 'room'

const App: React.FC = () => {
  const [view] = useState<AppView>('home')

  return (
    <ErrorBoundary>
      <div className="app">
        {view === 'home' && <HomePage />}
        {view === 'room' && <RoomPage />}
      </div>
    </ErrorBoundary>
  )
}

export default App
