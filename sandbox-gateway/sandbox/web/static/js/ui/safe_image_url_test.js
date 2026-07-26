import {strict as assert} from 'node:assert';
import {describe, it} from 'node:test';
import {isSafeImageURL} from './safe_image_url.js';

describe('isSafeImageURL', () => {
    it('allows data:image and http(s)', () => {
        assert.equal(isSafeImageURL('data:image/png;base64,AAA'), true);
        assert.equal(isSafeImageURL('data:image/jpeg,AAAA'), true);
        assert.equal(isSafeImageURL('https://example.com/a.png'), true);
        assert.equal(isSafeImageURL('http://example.com/a.png'), true);
    });

    it('rejects javascript and non-image data URLs', () => {
        assert.equal(isSafeImageURL('javascript:alert(1)'), false);
        assert.equal(isSafeImageURL('data:text/html,<script>'), false);
        assert.equal(isSafeImageURL('data:image/svg+xml;base64,PHN2Zy'), true);
        assert.equal(isSafeImageURL('//evil.example/x'), false);
        assert.equal(isSafeImageURL('/relative.png'), false);
        assert.equal(isSafeImageURL(''), false);
        assert.equal(isSafeImageURL(null), false);
    });
});
