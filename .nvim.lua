-- GICEL editor integration (project-local exrc)
-- Requires: Neovim 0.11+ with exrc enabled (set exrc in init.lua)

vim.filetype.add({ extension = { gicel = "gicel" } })

vim.api.nvim_create_autocmd("FileType", {
  pattern = "gicel",
  callback = function()
    vim.bo.commentstring = "-- %s"
    vim.bo.tabstop = 2
    vim.bo.shiftwidth = 2
    vim.bo.expandtab = true
  end,
})

vim.lsp.config("gicel", {
  cmd = { vim.fn.getcwd() .. "/bin/gicel", "lsp" },
  filetypes = { "gicel" },
  root_markers = { ".git" },
})
vim.lsp.enable("gicel")
