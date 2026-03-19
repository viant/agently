import React from 'react';

export const DetailContext = React.createContext({
  showDetail: () => {},
  closeDetail: () => {},
  setDetailMode: () => {},
  detailMode: 'right'
});
