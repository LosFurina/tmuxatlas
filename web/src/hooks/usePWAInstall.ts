import { useCallback, useEffect, useState } from 'react'

export type InstallOutcome = 'idle' | 'accepted' | 'dismissed' | 'installed'

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{
    outcome: 'accepted' | 'dismissed'
    platform: string
  }>
}

export interface PWAInstallState {
  canPrompt: boolean
  isStandalone: boolean
  isAppleMobile: boolean
  outcome: InstallOutcome
  install: () => Promise<InstallOutcome>
}

function standaloneMode(): boolean {
  const navigatorWithStandalone = navigator as Navigator & { standalone?: boolean }
  return window.matchMedia('(display-mode: standalone)').matches
    || navigatorWithStandalone.standalone === true
}

function appleMobileDevice(): boolean {
  const classicDevice = /iPad|iPhone|iPod/i.test(navigator.userAgent)
  const modernIPad = navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1
  return classicDevice || modernIPad
}

export function usePWAInstall(): PWAInstallState {
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null)
  const [isStandalone, setIsStandalone] = useState(standaloneMode)
  const [isAppleMobile] = useState(appleMobileDevice)
  const [outcome, setOutcome] = useState<InstallOutcome>(() => (
    standaloneMode() ? 'installed' : 'idle'
  ))

  useEffect(() => {
    const displayMode = window.matchMedia('(display-mode: standalone)')
    const updateStandalone = () => {
      const installed = standaloneMode()
      setIsStandalone(installed)
      if (installed) {
        setDeferredPrompt(null)
        setOutcome('installed')
      }
    }
    const capturePrompt = (event: Event) => {
      const installEvent = event as BeforeInstallPromptEvent
      event.preventDefault()
      setDeferredPrompt(installEvent)
      setOutcome('idle')
    }
    const markInstalled = () => {
      setDeferredPrompt(null)
      setIsStandalone(true)
      setOutcome('installed')
    }

    displayMode.addEventListener('change', updateStandalone)
    window.addEventListener('beforeinstallprompt', capturePrompt)
    window.addEventListener('appinstalled', markInstalled)
    updateStandalone()

    return () => {
      displayMode.removeEventListener('change', updateStandalone)
      window.removeEventListener('beforeinstallprompt', capturePrompt)
      window.removeEventListener('appinstalled', markInstalled)
    }
  }, [])

  const install = useCallback(async (): Promise<InstallOutcome> => {
    if (!deferredPrompt || isStandalone) {
      return isStandalone ? 'installed' : outcome
    }

    await deferredPrompt.prompt()
    const choice = await deferredPrompt.userChoice
    setDeferredPrompt(null)
    setOutcome(choice.outcome)
    return choice.outcome
  }, [deferredPrompt, isStandalone, outcome])

  return {
    canPrompt: deferredPrompt !== null && !isStandalone,
    isStandalone,
    isAppleMobile,
    outcome,
    install,
  }
}
