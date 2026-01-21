# Test Fixtures

This directory contains test fixtures for integration tests.

## Audio Fixtures

Place audio files in the `audio/` directory:

- `hello.wav` - A recording of "Hello" or "Hello, World!"
- `question.wav` - A recording of "What is two plus two?"
- `long_speech.wav` - A longer audio file for duration tests

Requirements:
- WAV format preferred (PCM, 16-bit, mono or stereo)
- Sample rate: 8kHz-48kHz
- Clear speech in English

## Image Fixtures

Place image files in the `images/` directory:

- `cat.png` - An image of a cat (for vision tests)
- `chart.png` - An image of a chart or graph
- `text_image.png` - An image containing readable text

Requirements:
- PNG or JPEG format
- Reasonable size (not too large)

## Document Fixtures

Place document files in the `documents/` directory:

- `sample.pdf` - A sample PDF document

## Generating Test Fixtures

If fixtures are not present, the test suite will use minimal placeholder
data that allows tests to run but may not produce meaningful results.

To generate real audio fixtures, you can use espeak or say:

```bash
# macOS
say -o audio/hello.wav "Hello, World!"
say -o audio/question.wav "What is two plus two?"

# Linux (with espeak)
espeak "Hello, World!" --stdout > audio/hello.wav
espeak "What is two plus two?" --stdout > audio/question.wav
```
