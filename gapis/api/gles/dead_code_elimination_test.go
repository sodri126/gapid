// Copyright (C) 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gles_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/gapid/core/assert"
	"github.com/google/gapid/core/log"
	"github.com/google/gapid/core/os/device"
	"github.com/google/gapid/core/os/device/bind"
	"github.com/google/gapid/gapis/api"
	"github.com/google/gapid/gapis/api/gles"
	"github.com/google/gapid/gapis/api/testcmd"
	"github.com/google/gapid/gapis/api/transform"
	"github.com/google/gapid/gapis/capture"
	"github.com/google/gapid/gapis/database"
	"github.com/google/gapid/gapis/memory"
	"github.com/google/gapid/gapis/resolve/dependencygraph"
)

func TestDeadCommandRemoval(t *testing.T) {
	ctx := log.Testing(t)
	ctx = bind.PutRegistry(ctx, bind.NewRegistry())
	ctx = database.Put(ctx, database.NewInMemory(ctx))

	// Keep the given command alive in the optimization.
	isLive := map[api.Cmd]bool{}
	live := func(cmd api.Cmd) api.Cmd { isLive[cmd] = true; return cmd }

	// Expect the command to be removed by the optimization.
	isDead := map[api.Cmd]bool{}
	dead := func(cmd api.Cmd) api.Cmd { isDead[cmd] = true; return cmd }

	programInfoA := &gles.LinkProgramExtra{
		LinkStatus: gles.GLboolean_GL_TRUE,
		ActiveResources: &gles.ActiveProgramResources{
			DefaultUniformBlock: gles.NewUniformIndexːProgramResourceʳᵐ().Add(0, &gles.ProgramResource{
				Name: "uniforms",
				Type: gles.GLenum_GL_FLOAT_VEC4,
				Locations: gles.NewU32ːGLintᵐ().
					Add(0, 0).Add(1, 1).Add(2, 2).Add(3, 3).Add(4, 4).
					Add(5, 5).Add(6, 6).Add(7, 7).Add(8, 8).Add(9, 9),
				ArraySize: 10,
			}),
		},
	}

	programInfoB := &gles.LinkProgramExtra{
		LinkStatus: gles.GLboolean_GL_TRUE,
		ActiveResources: &gles.ActiveProgramResources{
			DefaultUniformBlock: gles.NewUniformIndexːProgramResourceʳᵐ().Add(0, &gles.ProgramResource{
				Name:      "sampler",
				Type:      gles.GLenum_GL_SAMPLER_CUBE,
				Locations: gles.NewU32ːGLintᵐ().Add(0, 0),
				ArraySize: 1,
			}),
		},
	}

	ctxHandle1 := memory.BytePtr(1)
	ctxHandle2 := memory.BytePtr(2)
	displayHandle := memory.BytePtr(3)
	surfaceHandle := memory.BytePtr(4)
	cb := gles.CommandBuilder{Thread: 0}
	prologue := []api.Cmd{
		cb.EglCreateContext(displayHandle, surfaceHandle, surfaceHandle, memory.Nullptr, ctxHandle1),
		api.WithExtras(
			cb.EglMakeCurrent(displayHandle, surfaceHandle, surfaceHandle, ctxHandle1, 0),
			gles.NewStaticContextStateForTest(), gles.NewDynamicContextStateForTest(64, 64, false)),
		cb.GlCreateProgram(1),
		cb.GlCreateProgram(2),
		cb.GlCreateProgram(3),
		api.WithExtras(cb.GlLinkProgram(1), programInfoA),
		api.WithExtras(cb.GlLinkProgram(2), programInfoA),
		api.WithExtras(cb.GlLinkProgram(3), programInfoB),
		cb.GlUseProgram(1),
	}
	allBuffers := gles.GLbitfield_GL_COLOR_BUFFER_BIT | gles.GLbitfield_GL_DEPTH_BUFFER_BIT | gles.GLbitfield_GL_STENCIL_BUFFER_BIT
	tests := map[string][]api.Cmd{
		"Draw calls up to the requested point are preserved": {
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 1, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 2, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 3, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 4, 0)),
		},
		"No request in frame kills draw calls": {
			dead(cb.GlClear(allBuffers)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 1, 0)),
			dead(cb.EglSwapBuffers(displayHandle, surfaceHandle, gles.EGLBoolean(1))),
			cb.GlClear(allBuffers),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Multiple requests": {
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Simple overwrite": {
			dead(cb.GlUniform4fv(0, 1, memory.Nullptr)),
			cb.GlUniform4fv(1, 1, memory.Nullptr),
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			dead(cb.GlVertexAttribPointer(0, 4, gles.GLenum_GL_FLOAT, gles.GLboolean_GL_FALSE, 0, memory.Nullptr)),
			cb.GlVertexAttribPointer(1, 4, gles.GLenum_GL_FLOAT, gles.GLboolean_GL_FALSE, 0, memory.Nullptr),
			cb.GlVertexAttribPointer(0, 4, gles.GLenum_GL_FLOAT, gles.GLboolean_GL_FALSE, 0, memory.Nullptr),
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Overwrites should be tracked per program": {
			cb.GlUseProgram(1),
			dead(cb.GlUniform4fv(0, 1, memory.Nullptr)),
			cb.GlUseProgram(2),
			cb.GlUniform4fv(0, 1, memory.Nullptr), // Unaffected
			cb.GlUseProgram(1),
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			cb.GlUseProgram(1),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			cb.GlUseProgram(2),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Arrays should not interact with scalars": {
			cb.GlUniform4fv(0, 10, memory.Nullptr),
			cb.GlUniform4fv(0, 1, memory.Nullptr), // Unaffected
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Arrays should not interact with scalars (2)": {
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			cb.GlUniform4fv(0, 10, memory.Nullptr), // Unaffected
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Unsupported commands are left unmodified": {
			cb.GlUseProgram(1),
			dead(cb.GlUniform4fv(0, 1, memory.Nullptr)),
			cb.GlUniform1f(0, 3.14), // Not handled in the optimization.
			cb.GlLinkProgram(1),     // Not handled in the optimization.
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Multiple contexts": {
			// Draw in context 1
			dead(cb.GlUniform4fv(0, 1, memory.Nullptr)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			cb.GlClear(allBuffers),
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			// Draw in context 2
			cb.EglCreateContext(displayHandle, memory.Nullptr, memory.Nullptr, memory.Nullptr, ctxHandle2),
			api.WithExtras(
				cb.EglMakeCurrent(displayHandle, surfaceHandle, surfaceHandle, ctxHandle2, 0), gles.NewStaticContextStateForTest(), gles.NewDynamicContextStateForTest(64, 64, false)),
			cb.GlCreateProgram(1),
			api.WithExtras(cb.GlLinkProgram(1), programInfoA),
			cb.GlUseProgram(1),
			dead(cb.GlUniform4fv(0, 1, memory.Nullptr)),
			dead(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			cb.GlClear(allBuffers),
			cb.GlUniform4fv(0, 1, memory.Nullptr),
			cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0),
			// Request from both contexts
			cb.EglMakeCurrent(displayHandle, surfaceHandle, surfaceHandle, ctxHandle1, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
			cb.EglMakeCurrent(displayHandle, surfaceHandle, surfaceHandle, ctxHandle2, 0),
			live(cb.GlDrawArrays(gles.GLenum_GL_TRIANGLES, 0, 0)),
		},
		"Clear layers and read texture": {
			cb.GlActiveTexture(gles.GLenum_GL_TEXTURE3),
			cb.GlBindTexture(gles.GLenum_GL_TEXTURE_CUBE_MAP, 4),
			cb.GlTexStorage2D(gles.GLenum_GL_TEXTURE_CUBE_MAP, 10, gles.GLenum_GL_RGBA8, 512, 512),
			cb.GlActiveTexture(gles.GLenum_GL_TEXTURE0),

			cb.GlBindFramebuffer(gles.GLenum_GL_FRAMEBUFFER, 1),
			cb.GlFramebufferTexture2D(gles.GLenum_GL_FRAMEBUFFER, gles.GLenum_GL_COLOR_ATTACHMENT0, gles.GLenum_GL_TEXTURE_CUBE_MAP_NEGATIVE_X, 4, 0),
			cb.GlClear(gles.GLbitfield_GL_COLOR_BUFFER_BIT),
			cb.GlDrawArrays(gles.GLenum_GL_POINTS, 0, 1),

			cb.GlBindFramebuffer(gles.GLenum_GL_FRAMEBUFFER, 1),
			cb.GlFramebufferTexture2D(gles.GLenum_GL_FRAMEBUFFER, gles.GLenum_GL_COLOR_ATTACHMENT0, gles.GLenum_GL_TEXTURE_CUBE_MAP_NEGATIVE_Y, 4, 1),
			cb.GlClear(gles.GLbitfield_GL_COLOR_BUFFER_BIT),
			cb.GlDrawArrays(gles.GLenum_GL_POINTS, 0, 1),

			cb.GlBindFramebuffer(gles.GLenum_GL_FRAMEBUFFER, 1),
			cb.GlFramebufferTexture2D(gles.GLenum_GL_FRAMEBUFFER, gles.GLenum_GL_COLOR_ATTACHMENT0, gles.GLenum_GL_TEXTURE_CUBE_MAP_NEGATIVE_Z, 4, 2),
			cb.GlClear(gles.GLbitfield_GL_COLOR_BUFFER_BIT),
			cb.GlDrawArrays(gles.GLenum_GL_POINTS, 0, 1),

			cb.GlUseProgram(3),
			cb.GlUniform1i(0, 3),
			cb.GlBindFramebuffer(gles.GLenum_GL_FRAMEBUFFER, 0),
			cb.GlClear(gles.GLbitfield_GL_COLOR_BUFFER_BIT),
			live(cb.GlDrawArrays(gles.GLenum_GL_POINTS, 0, 1)),
		},
		"Generate mipmaps": {
			cb.GlActiveTexture(gles.GLenum_GL_TEXTURE0),
			cb.GlBindTexture(gles.GLenum_GL_TEXTURE_2D, 10),
			cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 0, gles.GLint(gles.GLenum_GL_RGB), 64, 64, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr),
			dead(cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 1, gles.GLint(gles.GLenum_GL_RGB), 32, 32, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr)),
			dead(cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 2, gles.GLint(gles.GLenum_GL_RGB), 16, 16, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr)),
			dead(cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 3, gles.GLint(gles.GLenum_GL_RGB), 8, 8, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr)),
			dead(cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 4, gles.GLint(gles.GLenum_GL_RGB), 4, 4, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr)),
			dead(cb.GlTexImage2D(gles.GLenum_GL_TEXTURE_2D, 5, gles.GLint(gles.GLenum_GL_RGB), 2, 2, 0, gles.GLenum_GL_RGB, gles.GLenum_GL_UNSIGNED_SHORT_5_6_5, memory.Nullptr)),
			live(cb.GlGenerateMipmap(gles.GLenum_GL_TEXTURE_2D)),
		},
	}

	for name, testCmds := range tests {
		cmds := append(prologue, testCmds...)

		h := &capture.Header{Abi: device.WindowsX86_64}
		capturePath, err := capture.New(ctx, name, h, cmds)
		if err != nil {
			panic(err)
		}
		ctx = capture.Put(ctx, capturePath)

		// First verify the commands mutate without errors
		c, _ := capture.Resolve(ctx)
		s := c.NewState(ctx)
		err = api.ForeachCmd(ctx, cmds, func(ctx context.Context, id api.CmdID, cmd api.Cmd) error {
			if err := cmd.Mutate(ctx, id, s, nil); err != nil {
				return fmt.Errorf("%v: %v: %v", id, cmd, err)
			}
			return nil
		})
		if !assert.For(ctx, "Test '%v' errors", name).ThatError(err).Succeeded() {
			continue
		}

		dependencyGraph, err := dependencygraph.GetDependencyGraph(ctx)
		if err != nil {
			t.Fatalf("%v", err)
		}
		transform := transform.NewDeadCodeElimination(ctx, dependencyGraph)

		expectedCmds := []api.Cmd{}
		api.ForeachCmd(ctx, cmds, func(ctx context.Context, id api.CmdID, cmd api.Cmd) error {
			if isLive[cmd] {
				transform.Request(id)
			}
			if !isDead[cmd] {
				expectedCmds = append(expectedCmds, cmd)
			}
			return nil
		})

		w := &testcmd.Writer{}
		transform.Flush(ctx, w)

		assert.For(ctx, "Test '%v'", name).ThatSlice(w.Cmds).Equals(expectedCmds)
	}
}
