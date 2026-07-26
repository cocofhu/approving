import {strict as assert} from 'node:assert';
import {describe, it} from 'node:test';
import {sanitizeImageURL} from './safe_image_url.js';

describe('sanitizeImageURL', () => {
    it('allows whitelisted data:image and returns a rebuilt safe string', () => {
        assert.equal(
            sanitizeImageURL('data:image/png;base64,AAA'),
            'data:image/png;base64,AAA',
        );
        assert.equal(
            sanitizeImageURL('data:image/jpeg,AAAA'),
            'data:image/jpeg,AAAA',
        );
        assert.equal(
            sanitizeImageURL('data:image/gif;base64,AAAA'),
            'data:image/gif;base64,AAAA',
        );
        assert.equal(
            sanitizeImageURL('data:image/webp;base64,AAAA'),
            'data:image/webp;base64,AAAA',
        );
        assert.equal(
            sanitizeImageURL('data:image/jpg;base64,AAAA'),
            'data:image/jpeg;base64,AAAA',
        );
        // Payload must be charset-copied; illegal base64 chars are rejected.
        assert.equal(sanitizeImageURL('data:image/png;base64,BBB='), 'data:image/png;base64,BBB=');
        assert.equal(sanitizeImageURL('data:image/png;base64,AA<>'), '');
    });

    it('allows http(s) and returns URL.href', () => {
        assert.equal(
            sanitizeImageURL('https://example.com/a.png'),
            'https://example.com/a.png',
        );
        assert.equal(
            sanitizeImageURL('http://example.com/a.png'),
            'http://example.com/a.png',
        );
    });

    it('rejects javascript, svg+xml, relative, and non-image data', () => {
        assert.equal(sanitizeImageURL('javascript:alert(1)'), '');
        assert.equal(sanitizeImageURL('data:text/html,<script>'), '');
        assert.equal(sanitizeImageURL('data:image/svg+xml;base64,PHN2Zy'), '');
        assert.equal(sanitizeImageURL('//evil.example/x'), '');
        assert.equal(sanitizeImageURL('/relative.png'), '');
        assert.equal(sanitizeImageURL(''), '');
        assert.equal(sanitizeImageURL(null), '');
    });
});
