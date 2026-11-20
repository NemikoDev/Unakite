import { useEffect, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// subscribes to a wails event and calls the handler when it fires
export function useWailsEvent<T>(event: string, handler: (payload: T) => void) {
    const saved = useRef(handler)
    useEffect(() => {
        saved.current = handler
    });
    useEffect(() => {
        EventsOn(event, (payload: T) => saved.current(payload))
    }, [event])
}
