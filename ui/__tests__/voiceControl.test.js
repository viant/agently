import {detectVoiceControl} from '../src/utils/voiceControl';

describe('detectVoiceControl', () => {
  const cases = [
    {
      name: 'submit phrase is detected and stripped',
      input: 'Please do X and submit it now',
      want: {action: 'submit', cleanedText: 'Please do X and'},
    },
    {
      name: 'cancel phrase is detected and stripped',
      input: 'Actually cancel it now',
      want: {action: 'cancel', cleanedText: 'Actually'},
    },
    {
      name: 'prefers cancel when both present',
      input: 'submit it now but cancel it now',
      want: {action: 'cancel', cleanedText: 'but'},
    },
    {
      name: 'no command returns original text',
      input: 'hello there',
      want: {action: '', cleanedText: 'hello there'},
    },
    {
      name: 'handles punctuation around phrase',
      input: 'Ok. (submit it now).',
      want: {action: 'submit', cleanedText: 'Ok.'},
    },
  ];

  cases.forEach((tc) => {
    test(tc.name, () => {
      expect(detectVoiceControl(tc.input)).toEqual(tc.want);
    });
  });
});

