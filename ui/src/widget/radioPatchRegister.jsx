// Radio widget event adapter patch: ensure onChange receives value, not event
// and updates the data source via adapter.set(value).
import { registerEventAdapter } from 'forge/runtime/binding';

try {
  registerEventAdapter('radio', {
    onChange: ({ adapter }) => (val) => {
      const v = (val && val.target && val.target.value) ? val.target.value : val;
      adapter.set(v);
    },
  });
  // eslint-disable-next-line no-console
  
} catch (e) {
  // eslint-disable-next-line no-console
  console.error('[agently] radioPatchRegister failed', e);
}
