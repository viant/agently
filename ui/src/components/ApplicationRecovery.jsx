import React from 'react';
import {useRouteError} from 'react-router-dom';
import {Button,Typography} from 'forge/components';
import './applicationRecovery.css';

export function isAssetLoadError(error) {
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Loading (?:CSS )?chunk .* failed|Unable to preload CSS/i.test(String(error?.message || ''));
}

export function ApplicationRecovery({error,onReload=()=>window.location.reload()}) {
  const asset=isAssetLoadError(error);
  return <section role="alert" className="app-application-recovery">
    <Typography as="h2" typeRole="section">{asset?'Reload the app':'Unable to display this page'}</Typography>
    <Typography as="p" typeRole="body">{asset?'Some application files could not load. The app may have been updated. Reload to retry.':'This page could not be displayed. Reload the app to try again.'}</Typography>
    <Button variant="secondary" onClick={onReload}>Reload app</Button>
  </section>;
}

// Keep an expired lazy chunk from unmounting the conversation and its draft.
// Other render failures continue to the route's ordinary recovery boundary.
export class AssetLoadBoundary extends React.Component {
  state={error:null};
  static getDerivedStateFromError(error){return {error};}
  render(){
    if(this.state.error){
      if(!isAssetLoadError(this.state.error))throw this.state.error;
      return <ApplicationRecovery error={this.state.error} onReload={this.props.onReload}/>;
    }
    return this.props.children;
  }
}

export function ApplicationRouteError(){return <ApplicationRecovery error={useRouteError()}/>;}
