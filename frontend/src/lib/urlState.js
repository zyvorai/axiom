import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

function readStored(storageKey) {
    try {
        return localStorage.getItem(storageKey);
    } catch {
        return null;
    }
}

function writeStored(storageKey, value) {
    try {
        localStorage.setItem(storageKey, value);
    } catch {
        /* private mode — the URL still carries the state */
    }
}

/**
 * Filter/search/sort state that lives in the URL (`?q=nginx&ns=default`) so a filtered view can
 * be bookmarked, shared and survives reload and back/forward.
 *
 * The URL value is the parameter when present, else (with `persist: true`) the last value
 * stored in localStorage, else `defaultValue`. Setting a value equal to the default removes the
 * parameter so URLs stay short. Updates replace the history entry (typing in a search box must
 * not create one history step per keystroke) and always start from the live `location.search`,
 * so several setters called in the same tick (applying a saved view) do not clobber one another.
 *
 * The returned value is held in local state and updated synchronously: the router applies URL
 * changes in a transition, and a controlled input fed straight from it would drop characters when
 * typing fast. Changes that come from elsewhere (back/forward, a link, the global namespace
 * changing) are copied into the local state; echoes of our own writes are not.
 */
export function useUrlState(key, defaultValue = "", { persist = false, storageKey } = {}) {
    const [params, setParams] = useSearchParams();
    const sKey = storageKey || `axiom-filter:${key}`;
    const raw = params.get(key);
    const fromUrl = raw !== null ? raw : (persist && readStored(sKey)) || defaultValue;

    const [value, setLocal] = useState(fromUrl);
    const latest = useRef(value);
    latest.current = value;
    const observed = useRef(fromUrl); // last URL-derived value seen
    const pending = useRef([]); // values we wrote that the router has not delivered back yet

    useEffect(() => {
        if (fromUrl === observed.current) return;
        observed.current = fromUrl;
        const i = pending.current.indexOf(fromUrl);
        if (i >= 0) {
            pending.current.splice(0, i + 1); // our own write coming back
            return;
        }
        pending.current = [];
        latest.current = fromUrl;
        setLocal(fromUrl);
    }, [fromUrl]);

    const setValue = useCallback(
        (next) => {
            const v = String(typeof next === "function" ? next(latest.current) : next ?? "");
            const effective = v === "" ? defaultValue : v;
            if (effective !== observed.current || pending.current.length > 0) pending.current.push(effective);
            latest.current = effective;
            setLocal(effective);
            const sp = new URLSearchParams(window.location.search);
            if (v === "" || v === defaultValue) sp.delete(key);
            else sp.set(key, v);
            setParams(sp, { replace: true });
            if (persist) writeStored(sKey, v);
        },
        [key, defaultValue, persist, sKey, setParams]
    );

    return [value, setValue];
}
