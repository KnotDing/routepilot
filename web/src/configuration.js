import {request} from './client';

// The backend commits the configuration before returning. Runtime activation
// is displayed separately by the workspace's configuration status notice.
export async function saveSettings(settings,onProgress){
 onProgress?.('submitting');
 return request('settings','PUT',settings);
}
