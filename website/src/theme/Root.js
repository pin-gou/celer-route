import React, { useEffect } from 'react';
import { useLocation } from '@docusaurus/router';
import mediumZoom from 'medium-zoom';

const ZOOM_SELECTOR = '.markdown img';

export default function Root({ children }) {
  const location = useLocation();

  useEffect(() => {
    const zoom = mediumZoom(ZOOM_SELECTOR, {
      background: 'rgba(0, 0, 0, 0.85)',
      margin: 24,
      scrollOffset: 0,
    });

    return () => {
      zoom.detach();
    };
  }, [location.pathname]);

  return <>{children}</>;
}
