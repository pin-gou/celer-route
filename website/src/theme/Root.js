import React, { useEffect, useRef } from 'react';
import { useLocation } from '@docusaurus/router';
import ExecutionEnvironment from '@docusaurus/ExecutionEnvironment';
import mediumZoom from 'medium-zoom';

const ZOOM_SELECTOR = '.markdown img';

// Module-level singleton. medium-zoom binds document-level click/scroll/
// keyup listeners on construction; recreating on every navigation would
// either leak listeners or briefly lose the click handler before the next
// page's images attach.
let zoomSingleton = null;
function getZoom() {
  if (!zoomSingleton) {
    zoomSingleton = mediumZoom(ZOOM_SELECTOR, {
      background: 'rgba(0, 0, 0, 0.85)',
      margin: 24,
      scrollOffset: 0,
    });
  }
  return zoomSingleton;
}

export default function Root({ children }) {
  const location = useLocation();
  const isFirstRunRef = useRef(true);

  useEffect(() => {
    if (!ExecutionEnvironment.canUseDOM) {
      return undefined;
    }

    const zoom = getZoom();
    let cancelled = false;

    // Docusaurus renders MDX content via React.lazy + Suspense, so the new
    // page's <img> nodes are not in the DOM when this effect runs. Even
    // rAF + rAF is not enough: the lazy chunk may load later, after our
    // attach, and React mounts new <img> elements with their default
    // className only. Watch the document for newly-added matching images
    // and re-attach the zoom to them. medium-zoom's `attach` is
    // idempotent (no-op for already-attached images), so re-running it on
    // every childList mutation is safe and cheap.
    const docObserver = new MutationObserver(() => {
      if (!cancelled) zoom.attach(ZOOM_SELECTOR);
    });
    docObserver.observe(document.body, { childList: true, subtree: true });

    if (isFirstRunRef.current) {
      // First mount: medium-zoom's constructor already attached every
      // currently-matching image. The observer will catch any future
      // additions (e.g. when an embedded React component renders an
      // <img> after the initial commit).
      isFirstRunRef.current = false;
    } else {
      // Subsequent navigation: synchronously attach to anything already
      // mounted, then let the observer pick up the rest.
      zoom.attach(ZOOM_SELECTOR);
    }

    return () => {
      cancelled = true;
      docObserver.disconnect();
    };
  }, [location.pathname]);

  return <>{children}</>;
}