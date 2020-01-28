/*
 * recovery.go - support for generating recovery passphrases
 *
 * Copyright 2019 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package actions

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pkg/errors"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/metadata"
)

// modifiedContextWithSource returns a copy of ctx with the protector source
// replaced by source.
func modifiedContextWithSource(ctx *Context, source metadata.SourceType) *Context {
	modifiedConfig := *ctx.Config
	modifiedConfig.Source = source
	modifiedCtx := *ctx
	modifiedCtx.Config = &modifiedConfig
	return &modifiedCtx
}

// AddRecoveryPassphrase randomly generates a recovery passphrase and adds it as
// a custom_passphrase protector for the given Policy.
func AddRecoveryPassphrase(policy *Policy, dirname string) (*crypto.Key, *Protector, error) {
	// 20 random characters in a-z is 94 bits of entropy, which is way more
	// than enough for a passphrase which still goes through the usual
	// passphrase hashing which makes it extremely costly to brute force.
	passphrase, err := crypto.NewRandomPassphrase(20)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			passphrase.Wipe()
		}
	}()
	getPassphraseFn := func(info ProtectorInfo, retry bool) (*crypto.Key, error) {
		// CreateProtector() wipes the passphrase, but in this case we
		// still need it for later, so make a copy.
		return passphrase.Clone()
	}
	var recoveryProtector *Protector
	customCtx := modifiedContextWithSource(policy.Context, metadata.SourceType_custom_passphrase)
	seq := 1
	for {
		// Automatically generate a name for the recovery protector.
		name := "Recovery passphrase for " + dirname
		if seq != 1 {
			name += " (" + strconv.Itoa(seq) + ")"
		}
		recoveryProtector, err = CreateProtector(customCtx, name, getPassphraseFn)
		if err == nil {
			break
		}
		if errors.Cause(err) != ErrDuplicateName {
			return nil, nil, err
		}
		seq++
	}
	if err := policy.AddProtector(recoveryProtector); err != nil {
		recoveryProtector.Revert()
		return nil, nil, err
	}
	return passphrase, recoveryProtector, nil
}

// WriteRecoveryInstructions writes a recovery passphrase and instructions to a
// file.  This file should initially be located in the encrypted directory
// protected by the passphrase itself.  It's up to the user to store the
// passphrase in a different location if they actually need it.
func WriteRecoveryInstructions(recoveryPassphrase *crypto.Key, path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	str := fmt.Sprintf(
		`fscrypt automatically generated a recovery passphrase for this directory:

    %s

It did this because you chose to protect this directory with your login
passphrase, but this directory is not on the root filesystem.

Copy this passphrase to a safe place if you want to still be able to unlock this
directory if you re-install your system or connect this storage media to a
different system (which would result in your login protector being lost).
`, recoveryPassphrase.Data())
	if _, err = file.WriteString(str); err != nil {
		return err
	}
	return file.Sync()
}
